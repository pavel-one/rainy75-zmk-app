#import <Foundation/Foundation.h>
#import <ApplicationServices/ApplicationServices.h>
#import <Carbon/Carbon.h>
#import <CoreAudio/CoreAudio.h>
#import <CoreAudio/AudioHardwareTapping.h>
#import <CoreAudio/CATapDescription.h>
#import <AudioToolbox/AudioServices.h>
#include <mach/mach.h>
#include <pthread.h>
#include <stdatomic.h>
#include <string.h>

#include "host_darwin.h"

// Реализована в host_darwin.go.
extern void rhGoKey(int vk);

// ---------- Перехват нажатий ----------

static CFMachPortRef tap;

static CGEventRef tapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *ref) {
    // Система выключает «медленный» перехват — включаем обратно.
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (tap) CGEventTapEnable(tap, true);
        return event;
    }
    int vk = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
    if (type == kCGEventKeyDown) {
        if (!CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat)) rhGoKey(vk);
        return event;
    }
    if (type != kCGEventFlagsChanged) return event;
    // Модификаторы приходят как смена флагов: нажатие — когда флаг поднят.
    CGEventFlags mask = 0;
    switch (vk) {
    case 56: case 60: mask = kCGEventFlagMaskShift; break;
    case 59: case 62: mask = kCGEventFlagMaskControl; break;
    case 58: case 61: mask = kCGEventFlagMaskAlternate; break;
    case 55: case 54: mask = kCGEventFlagMaskCommand; break;
    case 57: rhGoKey(vk); return event; // Caps Lock: любое нажатие меняет флаг
    default: return event;
    }
    if (CGEventGetFlags(event) & mask) rhGoKey(vk);
    return event;
}

static void *tapThread(void *arg) {
    CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(NULL, tap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), src, kCFRunLoopCommonModes);
    CGEventTapEnable(tap, true);
    CFRunLoopRun();
    return NULL;
}

int rh_keytap_allowed(void) { return CGPreflightListenEventAccess() ? 1 : 0; }

int rh_keytap_request(void) { return CGRequestListenEventAccess() ? 1 : 0; }

int rh_keytap_start(void) {
    if (tap) return 0;
    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventFlagsChanged);
    tap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionListenOnly,
                           mask, tapCallback, NULL);
    if (!tap) return -1;
    pthread_t th;
    pthread_create(&th, NULL, tapThread, NULL);
    pthread_detach(th);
    return 0;
}

// ---------- Раскладка и Caps Lock ----------

static char langBuf[32];
static _Atomic int langReady;

static void readLanguage(void) {
    TISInputSourceRef src = TISCopyCurrentKeyboardInputSource();
    if (!src) return;
    CFArrayRef langs = TISGetInputSourceProperty(src, kTISPropertyInputSourceLanguages);
    if (langs && CFArrayGetCount(langs) > 0) {
        NSString *lang = (__bridge NSString *)CFArrayGetValueAtIndex(langs, 0);
        strlcpy(langBuf, lang.UTF8String, sizeof(langBuf));
        atomic_store(&langReady, 1);
    }
    CFRelease(src);
}

char *rh_input_language(void) {
    // Text Input Sources требуют главного потока. Ждём его не дольше 500 мс:
    // если главный цикл занят или ещё не запущен, вернём прошлое значение.
    if ([NSThread isMainThread]) {
        readLanguage();
    } else {
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        dispatch_async(dispatch_get_main_queue(), ^{
            readLanguage();
            dispatch_semaphore_signal(done);
        });
        dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 500 * NSEC_PER_MSEC));
    }
    return atomic_load(&langReady) ? strdup(langBuf) : NULL;
}

int rh_caps_lock(void) {
    return (CGEventSourceFlagsState(kCGEventSourceStateHIDSystemState) & kCGEventFlagMaskAlphaShift) ? 1 : 0;
}

// ---------- Процессор и громкость ----------

int rh_cpu_ticks(unsigned long long *busy, unsigned long long *total) {
    host_cpu_load_info_data_t info;
    mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;
    if (host_statistics(mach_host_self(), HOST_CPU_LOAD_INFO, (host_info_t)&info, &count) != KERN_SUCCESS) return -1;
    unsigned long long user = info.cpu_ticks[CPU_STATE_USER], sys = info.cpu_ticks[CPU_STATE_SYSTEM];
    unsigned long long nice = info.cpu_ticks[CPU_STATE_NICE], idle = info.cpu_ticks[CPU_STATE_IDLE];
    *busy = user + sys + nice;
    *total = user + sys + nice + idle;
    return 0;
}

static AudioObjectID defaultOutput(void) {
    AudioObjectID dev = kAudioObjectUnknown;
    UInt32 size = sizeof(dev);
    AudioObjectPropertyAddress a = {kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal,
                                    kAudioObjectPropertyElementMain};
    AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, &dev);
    return dev;
}

int rh_volume(float *volume, int *muted) {
    AudioObjectID dev = defaultOutput();
    if (dev == kAudioObjectUnknown) return -1;
    Float32 v = 0;
    UInt32 size = sizeof(v);
    AudioObjectPropertyAddress va = {kAudioHardwareServiceDeviceProperty_VirtualMainVolume,
                                     kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain};
    if (AudioObjectGetPropertyData(dev, &va, 0, NULL, &size, &v) != noErr) {
        // У некоторых устройств общей громкости нет — берём первый канал.
        AudioObjectPropertyAddress ca = {kAudioDevicePropertyVolumeScalar, kAudioDevicePropertyScopeOutput, 1};
        size = sizeof(v);
        if (AudioObjectGetPropertyData(dev, &ca, 0, NULL, &size, &v) != noErr) return -2;
    }
    *volume = v;
    UInt32 m = 0;
    size = sizeof(m);
    AudioObjectPropertyAddress ma = {kAudioDevicePropertyMute, kAudioDevicePropertyScopeOutput,
                                     kAudioObjectPropertyElementMain};
    *muted = (AudioObjectGetPropertyData(dev, &ma, 0, NULL, &size, &m) == noErr && m) ? 1 : 0;
    return 0;
}

// ---------- Звук системы ----------

#define RING 8192
static float ring[RING];
static _Atomic unsigned int ringPos;
static AudioObjectID tapID = kAudioObjectUnknown, aggID = kAudioObjectUnknown;
static AudioDeviceIOProcID procID = NULL;
static double sampleRate = 48000;

static NSString *deviceUID(AudioObjectID dev) {
    CFStringRef uid = NULL;
    UInt32 size = sizeof(uid);
    AudioObjectPropertyAddress a = {kAudioDevicePropertyDeviceUID, kAudioObjectPropertyScopeGlobal,
                                    kAudioObjectPropertyElementMain};
    if (AudioObjectGetPropertyData(dev, &a, 0, NULL, &size, &uid) != noErr || !uid) return nil;
    return (__bridge_transfer NSString *)uid;
}

void rh_audio_stop(void) {
    if (aggID != kAudioObjectUnknown) {
        if (procID) {
            AudioDeviceStop(aggID, procID);
            AudioDeviceDestroyIOProcID(aggID, procID);
            procID = NULL;
        }
        AudioHardwareDestroyAggregateDevice(aggID);
        aggID = kAudioObjectUnknown;
    }
    if (tapID != kAudioObjectUnknown) {
        if (@available(macOS 14.2, *)) AudioHardwareDestroyProcessTap(tapID);
        tapID = kAudioObjectUnknown;
    }
}

int rh_audio_start(void) {
    if (aggID != kAudioObjectUnknown) return 0;
    if (@available(macOS 14.2, *)) {
        // Слушаем весь звук системы, ничего не заглушая.
        CATapDescription *desc = [[CATapDescription alloc] initStereoGlobalTapButExcludeProcesses:@[]];
        desc.name = @"Rainy75";
        desc.privateTap = YES;
        desc.muteBehavior = CATapUnmuted;
        OSStatus st = AudioHardwareCreateProcessTap(desc, &tapID);
        if (st != noErr) {
            tapID = kAudioObjectUnknown;
            return (int)st;
        }
        NSString *out = deviceUID(defaultOutput());
        if (!out) {
            rh_audio_stop();
            return -2;
        }
        NSDictionary *agg = @{
            @kAudioAggregateDeviceNameKey : @"Rainy75 Visualizer",
            @kAudioAggregateDeviceUIDKey : [[NSUUID UUID] UUIDString],
            @kAudioAggregateDeviceMainSubDeviceKey : out,
            @kAudioAggregateDeviceIsPrivateKey : @YES,
            @kAudioAggregateDeviceIsStackedKey : @NO,
            @kAudioAggregateDeviceTapAutoStartKey : @YES,
            @kAudioAggregateDeviceSubDeviceListKey : @[ @{@kAudioSubDeviceUIDKey : out} ],
            @kAudioAggregateDeviceTapListKey :
                @[ @{@kAudioSubTapDriftCompensationKey : @YES, @kAudioSubTapUIDKey : desc.UUID.UUIDString} ],
        };
        st = AudioHardwareCreateAggregateDevice((__bridge CFDictionaryRef)agg, &aggID);
        if (st != noErr) {
            aggID = kAudioObjectUnknown;
            rh_audio_stop();
            return (int)st;
        }
        AudioStreamBasicDescription fmt;
        UInt32 size = sizeof(fmt);
        AudioObjectPropertyAddress fa = {kAudioTapPropertyFormat, kAudioObjectPropertyScopeGlobal,
                                         kAudioObjectPropertyElementMain};
        if (AudioObjectGetPropertyData(tapID, &fa, 0, NULL, &size, &fmt) == noErr && fmt.mSampleRate > 0) {
            sampleRate = fmt.mSampleRate;
        }
        st = AudioDeviceCreateIOProcIDWithBlock(&procID, aggID, NULL,
            ^(const AudioTimeStamp *now, const AudioBufferList *in, const AudioTimeStamp *inTime,
              AudioBufferList *outData, const AudioTimeStamp *outTime) {
                if (!in || in->mNumberBuffers == 0) return;
                // Чередующиеся каналы в одном буфере или по буферу на канал — сводим в моно.
                const AudioBuffer *first = &in->mBuffers[0];
                UInt32 ch = first->mNumberChannels ? first->mNumberChannels : 1;
                UInt32 frames = first->mDataByteSize / sizeof(float) / ch;
                UInt32 nbuf = in->mNumberBuffers;
                unsigned int pos = atomic_load(&ringPos);
                for (UInt32 i = 0; i < frames; i++) {
                    float sum = 0;
                    UInt32 n = 0;
                    for (UInt32 b = 0; b < nbuf; b++) {
                        const AudioBuffer *buf = &in->mBuffers[b];
                        UInt32 bch = buf->mNumberChannels ? buf->mNumberChannels : 1;
                        if ((i + 1) * bch * sizeof(float) > buf->mDataByteSize) continue;
                        const float *d = buf->mData;
                        for (UInt32 c = 0; c < bch; c++, n++) sum += d[i * bch + c];
                    }
                    ring[pos % RING] = n ? sum / n : 0;
                    pos++;
                }
                atomic_store(&ringPos, pos);
            });
        if (st != noErr) {
            rh_audio_stop();
            return (int)st;
        }
        st = AudioDeviceStart(aggID, procID);
        if (st != noErr) {
            rh_audio_stop();
            return (int)st;
        }
        return 0;
    }
    return -100; // нужна macOS 14.2+
}

int rh_audio_read(float *out, int n) {
    if (n > RING) n = RING;
    unsigned int pos = atomic_load(&ringPos);
    for (int i = 0; i < n; i++) out[i] = ring[(pos - (unsigned int)n + (unsigned int)i) % RING];
    return n;
}

double rh_audio_rate(void) { return sampleRate; }
