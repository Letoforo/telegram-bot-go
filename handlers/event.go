package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"telegram-bot-go/models"
)

// Глобальная переменная для хранения активного ивента.
var currentEvent *EventDetails

// EventDetails хранит параметры и дату старта ивента.
type EventDetails struct {
	Name      string
	Oblomki   int
	Piastry   int
	StartDate time.Time
}

// StartEventCommand обрабатывает команду администратора для запуска ивента.
// Формат команды: "начатьивент Название ивента, число для обломков, число для пиастр"
func StartEventCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message, db *mongo.Database) {
	// Извлекаем параметры команды, удаляя ключевое слово.
	argsStr := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(message.Text), "начатьивент"))
	parts := strings.Split(argsStr, ",")
	if len(parts) < 3 {
		reply := tgbotapi.NewMessage(message.Chat.ID, "Неверный формат команды.\nИспользуйте: Начатьивент (Имя ивента), (число для обломков), (число для пиастр)")
		bot.Send(reply)
		return
	}
	eventName := strings.TrimSpace(parts[0])
	oblomkiStr := strings.TrimSpace(parts[1])
	piastryStr := strings.TrimSpace(parts[2])

	oblomki, err := strconv.Atoi(oblomkiStr)
	if err != nil {
		reply := tgbotapi.NewMessage(message.Chat.ID, "Ошибка: число для обломков указано неверно!")
		bot.Send(reply)
		return
	}
	piastry, err := strconv.Atoi(piastryStr)
	if err != nil {
		reply := tgbotapi.NewMessage(message.Chat.ID, "Ошибка: число для пиастр указано неверно!")
		bot.Send(reply)
		return
	}

	// Создаём объект ивента.
	currentEvent = &EventDetails{
		Name:      eventName,
		Oblomki:   oblomki,
		Piastry:   piastry,
		StartDate: time.Now(),
	}

	eventMessageText := fmt.Sprintf("Ивент %s Запущен!\nУчастникам, принявшим ивент, будет зачислено:\nОбломков: %d\nПиастр: %d\nДата начала: %s",
		currentEvent.Name, currentEvent.Oblomki, currentEvent.Piastry, currentEvent.StartDate.Format("02.01.2006 15:04"))

	// Создаём инлайн-клавиатуру с двумя кнопками.
	participateButton := tgbotapi.NewInlineKeyboardButtonData("Участвую", "event:participate")
	skipButton := tgbotapi.NewInlineKeyboardButtonData("Пропуск", "event:skip")
	inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(participateButton, skipButton),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, eventMessageText)
	msg.ReplyMarkup = inlineKeyboard
	bot.Send(msg)
}

// HandleEventCallback обрабатывает нажатия кнопок ивента.
func HandleEventCallback(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery, db *mongo.Database) {
	if currentEvent == nil {
		answer := tgbotapi.NewCallback(callback.ID, "Нет активного ивента.")
		bot.Send(answer)
		return
	}

	telegramUserID := callback.From.ID
	collection := db.Collection("users")
	filter := bson.M{"telegram_id": telegramUserID}

	// Получаем анкету пользователя из базы.
	var user models.UserProfile
	err := collection.FindOne(context.Background(), filter).Decode(&user)
	if err != nil {
		answer := tgbotapi.NewCallback(callback.ID, "Вы не зарегистрированы. Пожалуйста, пройдите регистрацию.")
		bot.Send(answer)
		return
	}

	eventDateStr := currentEvent.StartDate.Format("02.01.2006 15:04")
	data := callback.Data
	var responseText string

	if data == "event:participate" {
		// Выполняем обновление валюты в профиле, прибавляя заданную валюту.
		update := bson.M{"$inc": bson.M{"oblomki": currentEvent.Oblomki, "piastry": currentEvent.Piastry}}
		_, err := collection.UpdateOne(context.Background(), filter, update)
		if err != nil {
			answer := tgbotapi.NewCallback(callback.ID, "Ошибка обновления анкеты.")
			bot.Send(answer)
			return
		}

		// Перечитываем профиль, чтобы получить обновленные значения валюты.
		err = collection.FindOne(context.Background(), filter).Decode(&user)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(callback.Message.Chat.ID, "Ошибка получения обновленных данных."))
			return
		}

		responseText = fmt.Sprintf("%s\nРанг: %s\nКоманда: %s\nНовая сумма Обломков: %d (+%d)\nНовая сумма Пиастр: %d (+%d)\nДата начала ивента: %s",
			user.Name, user.Rank, user.Team, user.Oblomki, currentEvent.Oblomki, user.Piastry, currentEvent.Piastry, eventDateStr)
	} else if data == "event:skip" {
		responseText = fmt.Sprintf("%s\nРанг: %s\nКоманда: %s\nПропускает ивент\nДата начала ивента: %s",
			user.Name, user.Rank, user.Team, eventDateStr)
	} else {
		answer := tgbotapi.NewCallback(callback.ID, "Неизвестная команда!")
		bot.Send(answer)
		return
	}

	// Отправляем callback-ответ о принятии выбора.
	answer := tgbotapi.NewCallback(callback.ID, "Ваш выбор принят!")
	bot.Send(answer)

	// Отправляем сообщение с фотографией пользователя.
	photoMsg := tgbotapi.NewPhoto(callback.Message.Chat.ID, tgbotapi.FileID(user.PhotoFileID))
	photoMsg.Caption = responseText
	if _, err := bot.Send(photoMsg); err != nil {
		// Если отправка фотографии не удалась — отправляем текстовое сообщение.
		bot.Send(tgbotapi.NewMessage(callback.Message.Chat.ID, responseText))
	}
}
