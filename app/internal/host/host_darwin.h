// Системные данные macOS для подсветки Rainy75: нажатия клавиш, раскладка,
// Caps Lock, загрузка процессора, громкость, звук системы.
#pragma once

// Перехват нажатий (только чтение). Нужен доступ «Мониторинг ввода».
int rh_keytap_allowed(void);
int rh_keytap_request(void);
int rh_keytap_start(void); // 0 — запущен

// Язык текущей раскладки («en», «ru»…); строку освобождает вызывающий.
char *rh_input_language(void);

int rh_caps_lock(void);

// Счётчики процессора: занятые и все тики с загрузки системы.
int rh_cpu_ticks(unsigned long long *busy, unsigned long long *total);

// Громкость устройства вывода по умолчанию: 0…1 и признак «без звука».
int rh_volume(float *volume, int *muted);

// Звук системы через Core Audio Tap (macOS 14.2+). Нужен доступ «Запись звука системы».
int rh_audio_start(void); // 0 — запущен, иначе код ошибки Core Audio
void rh_audio_stop(void);
int rh_audio_read(float *out, int n); // последние n отсчётов моно
double rh_audio_rate(void);
