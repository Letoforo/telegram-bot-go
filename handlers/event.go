package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"telegram-bot-go/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/bson"
)

// DB – глобальная переменная для доступа к базе данных, инициализируется из main.go.

// InitHandlers сохраняет указатель на базу данных для использования в обработчиках.

// EventDetails хранит данные активного ивента.
type EventDetails struct {
	Name      string
	Oblomki   int
	Piastry   int
	StartDate time.Time
}

// currentEvent — глобальная переменная для текущего активного ивента.
var currentEvent *EventDetails

// HandleCreateEvent обрабатывает команду создания ивента.
// Ожидается формат команды: "начатьивент (Имя ивента), (число для обломков), (число для пиастр)"
func HandleCreateEvent(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	// Удаляем префикс "начатьивент" и приводим строку к нужному формату.
	argsStr := strings.TrimSpace(strings.TrimPrefix(message.Text, "начатьивент"))
	parts := strings.Split(argsStr, ",")
	if len(parts) < 3 {
		msg := tgbotapi.NewMessage(message.Chat.ID,
			"Неверный формат команды.\nИспользуйте: начатьивент (Имя ивента), (число для обломков), (число для пиастр)")
		bot.Send(msg)
		return
	}

	eventName := strings.TrimSpace(parts[0])
	oblomki, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка: число для обломков указано неверно."))
		return
	}
	piastry, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка: число для пиастр указано неверно."))
		return
	}

	// Сохраняем данные ивента в глобальной переменной.
	currentEvent = &EventDetails{
		Name:      eventName,
		Oblomki:   oblomki,
		Piastry:   piastry,
		StartDate: time.Now(),
	}

	eventMessage := fmt.Sprintf("Ивент '%s' запущен!\nУчастникам, принявшим ивент, будет зачислено:\nОбломков: %d\nПиастр: %d\nДата начала: %s",
		currentEvent.Name, currentEvent.Oblomki, currentEvent.Piastry, currentEvent.StartDate.Format("02.01.2006 15:04"))

	// Создаём инлайн-клавиатуру с кнопками "Участвую" и "Пропуск".
	participateButton := tgbotapi.NewInlineKeyboardButtonData("Участвую", "event:participate")
	skipButton := tgbotapi.NewInlineKeyboardButtonData("Пропуск", "event:skip")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(participateButton, skipButton),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, eventMessage)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// HandleEventCallback обрабатывает callback-запросы для кнопок ивента.
func HandleEventCallback(bot *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
	// Отправляем ответ на callback-запрос, чтобы кнопка перестала мигать.
	ack := tgbotapi.NewCallback(cq.ID, "")
	bot.Request(ack)

	// Проверяем, что база данных (DB) инициализирована.
	if DB == nil {
		answer := tgbotapi.NewCallback(cq.ID, "Ошибка сервера: база данных не инициализирована.")
		bot.Request(answer)
		return
	}

	// Проверяем, что активный ивент установлен.
	if currentEvent == nil {
		answer := tgbotapi.NewCallback(cq.ID, "Нет активного ивента.")
		bot.Request(answer)
		return
	}

	switch cq.Data {
	case "event:participate":
		usersColl := DB.Collection("users")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var user models.UserProfile
		err := usersColl.FindOne(ctx, bson.M{"telegram_id": int64(cq.From.ID)}).Decode(&user)
		if err != nil {
			answer := tgbotapi.NewCallback(cq.ID, "Профиль не найден. Зарегистрируйтесь, пожалуйста.")
			bot.Request(answer)
			return
		}

		// Начисляем валюту, указанную в ивенте.
		newPiastry := user.Piastry + currentEvent.Piastry
		newOblomki := user.Oblomki + currentEvent.Oblomki

		update := bson.M{
			"$set": bson.M{
				"piastry": newPiastry,
				"oblomki": newOblomki,
			},
		}

		_, err = usersColl.UpdateOne(ctx, bson.M{"telegram_id": int64(cq.From.ID)}, update)
		if err != nil {
			answer := tgbotapi.NewCallback(cq.ID, "Ошибка обновления профиля.")
			bot.Request(answer)
			return
		}

		answer := tgbotapi.NewCallback(cq.ID, "Успешно! Валюта зачислена в ваш профиль.")
		bot.Request(answer)
	case "event:skip":
		answer := tgbotapi.NewCallback(cq.ID, "Вы отказались от участия в ивенте.")
		bot.Request(answer)
	default:
		answer := tgbotapi.NewCallback(cq.ID, "Неверный выбор.")
		bot.Request(answer)
	}
}
