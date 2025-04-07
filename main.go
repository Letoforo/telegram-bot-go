package main

import (
	"log"
	"strings"

	"telegram-bot-go/db"
	"telegram-bot-go/handlers"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	// Токен бота (в продакшене рекомендуется использовать переменные окружения)
	token := "7547569544:AAHzaDCxeEqJCuEjWKwjAzXG_Q8ZTWjbQsk"

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("Ошибка создания бота: %v", err)
	}
	bot.Debug = true
	log.Printf("Запущен бот: %s", bot.Self.UserName)

	// Подключение к MongoDB
	mongoClient, err := db.ConnectMongo("mongodb://localhost:27017")
	if err != nil {
		log.Fatalf("Ошибка подключения к MongoDB: %v", err)
	}
	// Получаем базу данных
	database := mongoClient.Database("mydatabase")
	if database == nil {
		log.Fatal("Ошибка: База данных равна nil")
	}
	// Инициализируем обработчики, передав ссылку на базу данных
	handlers.InitHandlers(database)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			handlers.HandleCallbackQuery(bot, update.CallbackQuery)
			continue
		}

		if update.Message == nil {
			continue
		}

		// Если сообщение начинается со слэша, убираем слэш и упоминание бота.
		if strings.HasPrefix(update.Message.Text, "/") {
			cmd := strings.TrimPrefix(update.Message.Text, "/")
			if i := strings.Index(cmd, "@"); i != -1 {
				cmd = cmd[:i]
			}
			update.Message.Text = cmd

			// Приводим команду к нижнему регистру для унифицированной обработки.
			lowerCmd := strings.ToLower(cmd)
			if strings.HasPrefix(lowerCmd, "начатьивент") {
				handlers.HandleCreateEvent(bot, update.Message)
			} else if lowerCmd == "живой" ||
				lowerCmd == "список анкет" ||
				lowerCmd == "полный список анкет" ||
				strings.HasPrefix(lowerCmd, "анкета ") ||
				strings.HasPrefix(lowerCmd, "датьадмин") ||
				strings.HasPrefix(lowerCmd, "чек лог") {
				handlers.HandleAdminCommand(bot, update.Message)
			} else {
				handlers.HandleCommand(bot, update.Message)
			}
		} else {
			handlers.HandleNonCommandMessage(bot, update.Message)
		}
	}
}
