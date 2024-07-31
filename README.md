# Запуск

Проект упакован в docker. Все собарно в один docker-compose.yml. В окружении прописаны все настройки, включая пароль от PostgreSQL. Так лучше не делать для продакшена, здесь сделано для удобства развертывания тестового задания.

Запуск осуществялется через команду ```sh run.sh```

# API

## POST: api/message

Отправляет сообщение на app-main, там оно сохраняется в базу данных и отправляется в kafka. Далее из kafka сообщение читается app-consumer, затем app-consumer в БД помечает сообщение обработанными и записывает в БД статистику отправленных полльзователями сообщений.

**Body**:
```
{
    "userName": "nameTest",
    "text": "Тестовое сообщение"
}
```

**Response**: uuid сообщения

## GET: api/stats

app-main получает из базы данных статистику, ранее записанную туда app-consumer. Статистика представляет собой общее количество обработанных сообщений, а также количество обработанных сообщений каждого пользователя.

**Response**:
```
{
    "total_processed": 10,
    "processed_per_users": []
}
```

# Хостинг

ip: 62.217.179.214

62.217.179.214:3000/api/message

62.217.179.214:3000/api/stats


