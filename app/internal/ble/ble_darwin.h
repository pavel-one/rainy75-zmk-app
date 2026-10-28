// SMP (mcumgr) по Bluetooth LE через CoreBluetooth.
#pragma once

// Подключиться к клавиатуре, уже соединённой с Mac по Bluetooth: имя
// содержит name (без учёта регистра). 0 — готово, иначе код ошибки (rb_error).
int rb_connect(const char *name, int timeout_ms);
const char *rb_error(int code);

// Записать SMP-пакет (делится на части по MTU). 0 — успех.
int rb_write(const void *data, int len);

// Сколько байт уходит одной записью без ответа; 0 — нет связи.
int rb_max_write(void);

void rb_disconnect(void);
int rb_connected(void);
