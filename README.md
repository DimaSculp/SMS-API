# Тестовые запросы для SMS API Service

## 1. GET_SERVICES - Получение списка доступных сервисов

```PowerShell
(curl -Uri "http://176.124.200.52:8080/GrizzlySMSbyDima.php" -Method POST -Headers @{"Content-Type" = "application/json" ; "User-Agent" = "GrizzlySMS-Client/1.0"} -Body '{"action": "GET_SERVICES", "key": "qwerty123"}').Content
```

**Ожидаемый ответ:**
```json
{
  "status":"SUCCESS",
  "countryList":[
    {"country":"bel",
      "operatorMap":{
        "any":{"fb":57,"ok":57,"tg":57,"vk":57,"wa":57}}},
    {"country":"rus",
      "operatorMap":{
        "any":{"fb":51,"ok":51,"tg":51,"vk":51,"wa":51}}},
    {"country":"uzb","operatorMap"
    :{"any":{"fb":51,"ok":51,"tg":51,"vk":51,"wa":51}}}
  ]
}
```

## 2. GET_NUMBER - Получение номера телефона

```PowerShell
(curl -Uri "http://176.124.200.52:8080/GrizzlySMSbyDima.php" -Method POST -Headers @{"Content-Type" = "application/json" ; "User-Agent" = "GrizzlySMS-Client/1.0"} -Body '{"action": "GET_NUMBER", "key": "qwerty123", "country": "rus", "operator": "any", "service": "tg", "sum": 20.00}').Content
```

**Ожидаемый ответ:**
```json
{
  "status": "SUCCESS",
  "number": 79157891133,
  "activationId": 1,
  "flashcall": true
}
```

## 3. GET_NUMBER с исключающими префиксами

```PowerShell
(curl -Uri "http://176.124.200.52:8080/GrizzlySMSbyDima.php" -Method POST -Headers @{"Content-Type" = "application/json" ; "User-Agent" = "GrizzlySMS-Client/1.0"} -Body '{"action": "GET_NUMBER", "key": "qwerty123", "country": "rus", "operator": "any", "service": "tg", "sum": 20.00, "exceptionPhoneSet": ["7918", "79281"]}').Content
```

**Ожидаемый ответ:**
```json
{
  "status": "SUCCESS",
  "number": 79157891133,
  "activationId": 1,
  "flashcall": true
}
```

## 4. PUSH_SMS - Отправка SMS

```PowerShell
(curl -Uri "http://176.124.200.52:8080/GrizzlySMSbyDima.php" -Method POST -Headers @{"Content-Type" = "application/json"; "User-Agent" = "GrizzlySMS-Client/1.0"} -Body '{"action": "PUSH_SMS", "key": "qwerty123", "activationId": 1, "sms": "Your code: 123456"}').content
```

**Ожидаемый ответ:**
```json
{
  "status": "SUCCESS"
}
```

## 5. FINISH_ACTIVATION - Завершение активации

```PowerShell
(curl -Uri "http://176.124.200.52:8080/GrizzlySMSbyDima.php" -Method POST -Headers @{"Content-Type" = "application/json"; "User-Agent" = "GrizzlySMS-Client/1.0"} -Body '{"action": "FINISH_ACTIVATION", "key": "qwerty123", "activationId": 1, "status": 3}').content
```

**Ожидаемый ответ:**
```json
{
  "status": "SUCCESS"
}
```

## Тест с неверным ключом

```PowerShell
(curl -Uri "http://176.124.200.52:8080/GrizzlySMSbyDima.php" -Method POST -Headers @{"Content-Type" = "application/json"; "User-Agent" = "GrizzlySMS-Client/1.0"} -Body '{"action": "GET_SERVICE", "key" : "123"}').content
```

**Ожидаемый ответ:**
```json
{
  "status": "INVALID_KEY"
}
```

## Возможные статусы ответов

- `SUCCESS` - Операция выполнена успешно
- `INVALID_KEY` - Неверный API ключ
- `INVALID_ACTION` - Неизвестное действие
- `INVALID_REQUEST` - Неверный формат запроса
- `NO_NUMBERS` - Нет доступных номеров
- `ACTIVATION_NOT_FOUND` - Активация не найдена
- `DATABASE_ERROR` - Ошибка базы данных
