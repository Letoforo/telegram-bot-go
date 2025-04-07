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

func HandleEventCallback(bot *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
	// Отправляем ответ на callback-запрос, чтобы кнопка перестала мигать.
	ack := tgbotapi.NewCallback(cq.ID, "")
	_, _ = bot.Request(ack)

	// Проверяем, что активный ивент установлен.
	if currentEvent == nil {
		bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Нет активного ивента."))
		return
	}

	// Создаем контекст с timeout для операций с базой.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Формируем фильтр для поиска пользователя по Telegram ID.
	filter := bson.M{"telegram_id": int64(cq.From.ID)}

	// Объявляем переменную для хранения профиля.
	var profile models.UserProfile

	// Сначала пробуем получить профиль из базы.
	err := userCollection.FindOne(ctx, filter).Decode(&profile)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID,
			"Анкета не найдена. Зарегистрируйтесь командой: регистрация"))
		return
	}

	if cq.Data == "event:participate" {
		// Опция «Участвую»: обновляем баланс валют пользователя.
		newPiastry := profile.Piastry + currentEvent.Piastry
		newOblomki := profile.Oblomki + currentEvent.Oblomki

		update := bson.M{"$set": bson.M{"piastry": newPiastry, "oblomki": newOblomki}}
		_, err = userCollection.UpdateOne(ctx, filter, update)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Ошибка обновления профиля."))
			return
		}
		// Считываем обновленный профиль.
		err = userCollection.FindOne(ctx, filter).Decode(&profile)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Ошибка получения обновленных данных профиля."))
			return
		}
		// Формируем строку с данными анкеты и информацией об ивенте.
		caption := fmt.Sprintf(
			"Имя: %s\nРаса: %s\nВозраст: %s\nРост и вес: %s\nПол: %s\nРанг: %s\nКоманда: %s\nОбломки: %d\nПиастры: %d\nИнвентарь: %s\n\nДата ивента: %s\nСтатус: Участвует",
			profile.Name, profile.Race, profile.Age, profile.HeightWeight,
			profile.Gender, profile.Rank, profile.Team, profile.Oblomki,
			profile.Piastry, profile.Inventory, currentEvent.StartDate.Format("02.01.2006 15:04"))
		// Отправляем фото с подписью.
		photoMsg := tgbotapi.NewPhoto(cq.Message.Chat.ID, tgbotapi.FileID(profile.PhotoFileID))
		photoMsg.Caption = caption
		bot.Send(photoMsg)
	} else if cq.Data == "event:skip" {
		// Опция «Пропуск»: баланс не обновляем, выводим статус пропуска.
		caption := fmt.Sprintf(
			"Имя: %s\nРаса: %s\nВозраст: %s\nРост и вес: %s\nПол: %s\nРанг: %s\nКоманда: %s\nОбломки: %d\nПиастры: %d\nИнвентарь: %s\n\nДата ивента: %s\nСтатус: Пропускает ивент",
			profile.Name, profile.Race, profile.Age, profile.HeightWeight,
			profile.Gender, profile.Rank, profile.Team, profile.Oblomki,
			profile.Piastry, profile.Inventory, currentEvent.StartDate.Format("02.01.2006 15:04"))
		photoMsg := tgbotapi.NewPhoto(cq.Message.Chat.ID, tgbotapi.FileID(profile.PhotoFileID))
		photoMsg.Caption = caption
		bot.Send(photoMsg)
	} else {
		bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Неверный выбор."))
	}
}
