#import <Foundation/Foundation.h>
#import <CoreBluetooth/CoreBluetooth.h>

#include "ble_darwin.h"

// Реализованы в ble_darwin.go.
extern void rbGoData(void *data, int len);
extern void rbGoDisconnected(void);

// GATT-сервис и характеристика SMP (mcumgr).
static NSString *const kSMPService = @"8D53DC1D-1DB7-4CD3-868B-8A527460AA84";
static NSString *const kSMPChar = @"DA2E7828-FBCE-4E01-AE9E-261174997C48";
static NSString *const kHIDService = @"1812";

enum {
    RB_OK = 0,
    RB_ERR_UNAUTHORIZED = 1,
    RB_ERR_POWERED_OFF = 2,
    RB_ERR_UNSUPPORTED = 3,
    RB_ERR_NOT_FOUND = 4,
    RB_ERR_CONNECT = 5,
    RB_ERR_NO_SMP = 6,
    RB_ERR_TIMEOUT = 7,
    RB_ERR_NOT_CONNECTED = 8,
};

@interface RBLink : NSObject <CBCentralManagerDelegate, CBPeripheralDelegate>
@property(nonatomic, strong) CBCentralManager *central;
@property(nonatomic, strong) CBPeripheral *peripheral;
@property(nonatomic, strong) CBCharacteristic *smp;
@property(nonatomic, strong) dispatch_queue_t queue;
@property(nonatomic, strong) dispatch_semaphore_t stateSem;
@property(nonatomic, strong) dispatch_semaphore_t stepSem;
@property(nonatomic, strong) dispatch_semaphore_t writeSem;
@property(nonatomic) int stepCode;
@end

@implementation RBLink

- (instancetype)init {
    if ((self = [super init])) {
        _queue = dispatch_queue_create("one.pavel.rainy75.ble", DISPATCH_QUEUE_SERIAL);
        _stateSem = dispatch_semaphore_create(0);
        _stepSem = dispatch_semaphore_create(0);
        _writeSem = dispatch_semaphore_create(0);
        _central = [[CBCentralManager alloc] initWithDelegate:self queue:_queue options:nil];
    }
    return self;
}

- (void)centralManagerDidUpdateState:(CBCentralManager *)central {
    if (central.state != CBManagerStateUnknown && central.state != CBManagerStateResetting) {
        dispatch_semaphore_signal(self.stateSem);
    }
    if (central.state != CBManagerStatePoweredOn && self.peripheral) {
        self.peripheral = nil;
        self.smp = nil;
        rbGoDisconnected();
    }
}

// step завершает очередной шаг подключения с кодом code.
- (void)step:(int)code {
    self.stepCode = code;
    dispatch_semaphore_signal(self.stepSem);
}

- (void)centralManager:(CBCentralManager *)c didConnectPeripheral:(CBPeripheral *)p {
    [p discoverServices:@[ [CBUUID UUIDWithString:kSMPService] ]];
}

- (void)centralManager:(CBCentralManager *)c didFailToConnectPeripheral:(CBPeripheral *)p error:(NSError *)e {
    [self step:RB_ERR_CONNECT];
}

- (void)centralManager:(CBCentralManager *)c didDisconnectPeripheral:(CBPeripheral *)p error:(NSError *)e {
    if (p == self.peripheral) {
        self.peripheral = nil;
        self.smp = nil;
        rbGoDisconnected();
    }
}

- (void)peripheral:(CBPeripheral *)p didDiscoverServices:(NSError *)e {
    for (CBService *s in p.services) {
        if ([s.UUID isEqual:[CBUUID UUIDWithString:kSMPService]]) {
            [p discoverCharacteristics:@[ [CBUUID UUIDWithString:kSMPChar] ] forService:s];
            return;
        }
    }
    [self step:RB_ERR_NO_SMP];
}

- (void)peripheral:(CBPeripheral *)p didDiscoverCharacteristicsForService:(CBService *)s error:(NSError *)e {
    for (CBCharacteristic *ch in s.characteristics) {
        if ([ch.UUID isEqual:[CBUUID UUIDWithString:kSMPChar]]) {
            self.smp = ch;
            [p setNotifyValue:YES forCharacteristic:ch];
            return;
        }
    }
    [self step:RB_ERR_NO_SMP];
}

- (void)peripheral:(CBPeripheral *)p didUpdateNotificationStateForCharacteristic:(CBCharacteristic *)ch
             error:(NSError *)e {
    [self step:(e == nil && ch.isNotifying) ? RB_OK : RB_ERR_CONNECT];
}

- (void)peripheral:(CBPeripheral *)p didUpdateValueForCharacteristic:(CBCharacteristic *)ch error:(NSError *)e {
    if (e || ch != self.smp) return;
    NSData *d = ch.value;
    if (d.length) rbGoData((void *)d.bytes, (int)d.length);
}

- (void)peripheralIsReadyToSendWriteWithoutResponse:(CBPeripheral *)p {
    dispatch_semaphore_signal(self.writeSem);
}

- (CBPeripheral *)findPeripheral:(NSString *)name {
    // Клавиатура уже соединена с Mac как HID-устройство: ищем среди
    // подключённых, сначала по сервису SMP, затем по HID.
    for (NSString *svc in @[ kSMPService, kHIDService ]) {
        NSArray<CBPeripheral *> *list =
            [self.central retrieveConnectedPeripheralsWithServices:@[ [CBUUID UUIDWithString:svc] ]];
        for (CBPeripheral *p in list) {
            if (name.length == 0 || [p.name rangeOfString:name options:NSCaseInsensitiveSearch].location != NSNotFound) {
                return p;
            }
        }
    }
    return nil;
}

@end

static RBLink *link_;

static dispatch_time_t after(int ms) { return dispatch_time(DISPATCH_TIME_NOW, (int64_t)ms * NSEC_PER_MSEC); }

const char *rb_error(int code) {
    switch (code) {
    case RB_ERR_UNAUTHORIZED: return "нет разрешения на Bluetooth: Системные настройки → Конфиденциальность и безопасность → Bluetooth";
    case RB_ERR_POWERED_OFF: return "Bluetooth выключен";
    case RB_ERR_UNSUPPORTED: return "Bluetooth LE не поддерживается";
    case RB_ERR_NOT_FOUND: return "клавиатура не подключена к Mac по Bluetooth";
    case RB_ERR_CONNECT: return "не удалось подключиться к клавиатуре по Bluetooth";
    case RB_ERR_NO_SMP: return "в прошивке нет сервиса SMP по Bluetooth";
    case RB_ERR_TIMEOUT: return "клавиатура не ответила по Bluetooth";
    case RB_ERR_NOT_CONNECTED: return "нет связи по Bluetooth";
    default: return "ошибка Bluetooth";
    }
}

int rb_connect(const char *cname, int timeout_ms) {
    if (!link_) link_ = [[RBLink alloc] init];
    RBLink *l = link_;
    if (l.central.state == CBManagerStateUnknown || l.central.state == CBManagerStateResetting) {
        if (dispatch_semaphore_wait(l.stateSem, after(timeout_ms)) != 0) return RB_ERR_TIMEOUT;
    }
    switch (l.central.state) {
    case CBManagerStatePoweredOn: break;
    case CBManagerStateUnauthorized: return RB_ERR_UNAUTHORIZED;
    case CBManagerStatePoweredOff: return RB_ERR_POWERED_OFF;
    default: return RB_ERR_UNSUPPORTED;
    }
    if (rb_connected()) return RB_OK;

    NSString *name = cname ? [NSString stringWithUTF8String:cname] : @"";
    __block CBPeripheral *p = nil;
    dispatch_sync(l.queue, ^{ p = [l findPeripheral:name]; });
    if (!p) return RB_ERR_NOT_FOUND;

    while (dispatch_semaphore_wait(l.stepSem, DISPATCH_TIME_NOW) == 0) {} // сбросить старые сигналы
    dispatch_sync(l.queue, ^{
        l.peripheral = p;
        l.smp = nil;
        p.delegate = l;
        [l.central connectPeripheral:p options:nil];
    });
    if (dispatch_semaphore_wait(l.stepSem, after(timeout_ms)) != 0) {
        rb_disconnect();
        return RB_ERR_TIMEOUT;
    }
    if (l.stepCode != RB_OK) {
        int code = l.stepCode;
        rb_disconnect();
        return code;
    }
    return RB_OK;
}

int rb_write(const void *data, int len) {
    RBLink *l = link_;
    if (!l) return RB_ERR_NOT_CONNECTED;
    __block int rc = RB_OK;
    NSData *all = [NSData dataWithBytes:data length:len];
    NSUInteger off = 0;
    while (off < all.length) {
        __block BOOL ready = NO;
        __block NSUInteger chunk = 20;
        dispatch_sync(l.queue, ^{
            if (!l.peripheral || !l.smp || l.peripheral.state != CBPeripheralStateConnected) {
                rc = RB_ERR_NOT_CONNECTED;
                return;
            }
            chunk = MAX((NSUInteger)20, [l.peripheral maximumWriteValueLengthForType:CBCharacteristicWriteWithoutResponse]);
            ready = l.peripheral.canSendWriteWithoutResponse;
        });
        if (rc != RB_OK) return rc;
        if (!ready) {
            // Очередь передачи полна — ждём, пока CoreBluetooth её разгрузит.
            if (dispatch_semaphore_wait(l.writeSem, after(1000)) != 0) return RB_ERR_TIMEOUT;
            continue;
        }
        NSData *part = [all subdataWithRange:NSMakeRange(off, MIN(chunk, all.length - off))];
        dispatch_sync(l.queue, ^{
            [l.peripheral writeValue:part forCharacteristic:l.smp type:CBCharacteristicWriteWithoutResponse];
        });
        off += part.length;
    }
    return rc;
}

void rb_disconnect(void) {
    RBLink *l = link_;
    if (!l) return;
    dispatch_sync(l.queue, ^{
        if (l.peripheral) {
            // Закрываем только своё соединение: HID-соединение системы остаётся.
            if (l.smp && l.smp.isNotifying) [l.peripheral setNotifyValue:NO forCharacteristic:l.smp];
            [l.central cancelPeripheralConnection:l.peripheral];
        }
        l.peripheral = nil;
        l.smp = nil;
    });
}

int rb_connected(void) {
    RBLink *l = link_;
    if (!l) return 0;
    __block int ok = 0;
    dispatch_sync(l.queue, ^{
        ok = l.peripheral && l.smp && l.peripheral.state == CBPeripheralStateConnected;
    });
    return ok;
}
