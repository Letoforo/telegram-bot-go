package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	// Импорт MongoDB пакетов.
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"telegram-bot-go/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const PermanentAdminUsername = "My_Beautifu1_Madness"
const AdminEmoji = "🏴‍☠️"

// MarkUserAsAdmin назначает пользователя администратором:
// добавляет эмодзи к имени (если отсутствует) и устанавливает флаг is_admin.
func MarkUserAsAdmin(telegramID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"telegram_id": telegramID}
	var profile models.UserProfile
	err := userCollection.FindOne(ctx, filter).Decode(&profile)
	if err != nil {
		return err
	}
	if !strings.Contains(profile.Name, AdminEmoji) {
		profile.Name = profile.Name + " " + AdminEmoji
	}
	update := bson.M{"$set": bson.M{"is_admin": true, "name": profile.Name}}
	_, err = userCollection.UpdateOne(ctx, filter, update)
	return err
}

// IsUserAdmin возвращает true, если отправитель сообщения является администратором.
func IsUserAdmin(message *tgbotapi.Message) bool {
	if strings.EqualFold(message.From.UserName, PermanentAdminUsername) {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"telegram_id": message.From.ID}
	var profile models.UserProfile
	err := userCollection.FindOne(ctx, filter).Decode(&profile)
	return err == nil && profile.IsAdmin
}

// listProfiles выводит краткий список анкет в формате:
// "Айди анкеты, имя, юз пользователя, ранг, команда"
func listProfiles(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cursor, err := userCollection.Find(ctx, bson.M{})
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка при получении списка анкет."))
		return
	}
	defer cursor.Close(ctx)
	var result strings.Builder
	for cursor.Next(ctx) {
		var profile models.UserProfile
		if err := cursor.Decode(&profile); err != nil {
			continue
		}
		result.WriteString(fmt.Sprintf("ID: %s | Имя: %s | Username: @%s | Ранг: %s | Команда: %s\n",
			profile.ID.Hex(), profile.Name, profile.Username, profile.Rank, profile.Team))
	}
	if result.Len() == 0 {
		result.WriteString("Нет анкет.")
	}
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, result.String()))
}

// fullListProfiles выводит каждую анкету в отдельном сообщении (с фотографией, если имеется).
func fullListProfiles(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cursor, err := userCollection.Find(ctx, bson.M{})
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка при получении полного списка анкет."))
		return
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var profile models.UserProfile
		if err := cursor.Decode(&profile); err != nil {
			continue
		}
		caption := fmt.Sprintf(
			"Имя: %s\nРаса: %s\nВозраст: %s\nРост и вес: %s\nПол: %s\nРанг: %s\nКоманда: %s\nОбломки: %d\nПиастры: %d\nИнвентарь: %s",
			profile.Name, profile.Race, profile.Age, profile.HeightWeight,
			profile.Gender, profile.Rank, profile.Team, profile.Oblomki,
			profile.Piastry, profile.Inventory,
		)
		if profile.PhotoFileID != "" {
			photoMsg := tgbotapi.NewPhoto(message.Chat.ID, tgbotapi.FileID(profile.PhotoFileID))
			photoMsg.Caption = caption
			bot.Send(photoMsg)
		} else {
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, caption))
		}
	}
}

// showProfileByID выводит одну анкету по заданному ID.
func showProfileByID(bot *tgbotapi.BotAPI, message *tgbotapi.Message, profileID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	objID, err := primitive.ObjectIDFromHex(profileID)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверный формат айди анкеты."))
		return
	}
	filter := bson.M{"_id": objID}
	var profile models.UserProfile
	err = userCollection.FindOne(ctx, filter).Decode(&profile)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Анкета с указанным ID не найдена."))
		return
	}
	caption := fmt.Sprintf(
		"Имя: %s\nРаса: %s\nВозраст: %s\nРост и вес: %s\nПол: %s\nРанг: %s\nКоманда: %s\nОбломки: %d\nПиастры: %d\nИнвентарь: %s",
		profile.Name, profile.Race, profile.Age, profile.HeightWeight,
		profile.Gender, profile.Rank, profile.Team, profile.Oblomki,
		profile.Piastry, profile.Inventory,
	)
	if profile.PhotoFileID != "" {
		photoMsg := tgbotapi.NewPhoto(message.Chat.ID, tgbotapi.FileID(profile.PhotoFileID))
		photoMsg.Caption = caption
		bot.Send(photoMsg)
	} else {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, caption))
	}
}

// handleCheckLog обрабатывает команду "чек лог (день/неделя/месяц)"
// и выводит лог записей из коллекции logs за указанный период.
func handleCheckLog(bot *tgbotapi.BotAPI, message *tgbotapi.Message, duration time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	since := time.Now().Add(-duration)
	filter := bson.M{"date": bson.M{"$gte": since}}
	cursor, err := logsCollection.Find(ctx, filter)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка при получении логов."))
		return
	}
	defer cursor.Close(ctx)
	var result strings.Builder
	for cursor.Next(ctx) {
		var event struct {
			Date         time.Time `bson:"date"`
			Name         string    `bson:"name"`
			Username     string    `bson:"username"`
			ChangeAmount int       `bson:"change_amount"`
			Resource     string    `bson:"resource"`
		}
		if err := cursor.Decode(&event); err != nil {
			continue
		}
		dateStr := event.Date.Format("02.01.2006 15:04")
		result.WriteString(fmt.Sprintf("%s, %s, @%s, %d, %s\n", dateStr, event.Name, event.Username, event.ChangeAmount, event.Resource))
	}
	logText := result.String()
	if strings.TrimSpace(logText) == "" {
		logText = "Нет логов за выбранный период."
	}
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, logText))
}

// HandleAdminCommand обрабатывает админ-команды:
// "список анкет", "полный список анкет", "анкета (айди анкеты)",
// "датьадмин @username", "живой", "чек лог (день/неделя/месяц)".
func HandleAdminCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	if !IsUserAdmin(message) {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "У вас нет прав для выполнения этой команды."))
		return
	}

	// Получаем команду и приводим её к нижнему регистру.
	cmd := strings.TrimSpace(message.Text)
	lowerCmd := strings.ToLower(cmd)
	// Нормализуем команду, удаляя все пробелы — это позволит распознать и
	// команды вида "начать ивент" так же, как "начатьивент"
	normalizedCmd := strings.ReplaceAll(lowerCmd, " ", "")

	switch {
	case strings.HasPrefix(normalizedCmd, "начатьивент"):
		// Вызываем обработчик создания ивента.
		HandleCreateEvent(bot, message)
	case strings.EqualFold(lowerCmd, "живой"):
		resetRegistrationSessions()
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "сэр, да, сэр!"))
	case strings.EqualFold(lowerCmd, "список анкет"):
		listProfiles(bot, message)
	case strings.EqualFold(lowerCmd, "полный список анкет"):
		fullListProfiles(bot, message)
	case strings.HasPrefix(lowerCmd, "анкета "):
		parts := strings.Fields(cmd)
		if len(parts) < 2 {
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Укажите айди анкеты. Пример: анкета 603c2f..."))
			return
		}
		showProfileByID(bot, message, parts[1])
	case strings.HasPrefix(lowerCmd, "датьадмин"):
		parts := strings.Fields(message.Text)
		if len(parts) < 2 {
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверный формат команды. Пример: датьадмин @username"))
			return
		}
		targetUsername := strings.TrimPrefix(parts[1], "@")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		filter := bson.M{"username": targetUsername}
		var profile models.UserProfile
		err := userCollection.FindOne(ctx, filter).Decode(&profile)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Пользователь не найден или не зарегистрирован."))
			return
		}
		err = MarkUserAsAdmin(profile.TelegramID)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка при назначении администратора."))
			return
		}
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Пользователь @%s назначен администратором.", targetUsername)))
	case strings.HasPrefix(lowerCmd, "чек лог"):
		parts := strings.Fields(cmd)
		if len(parts) < 3 {
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Укажите период для логов: день, неделя или месяц. Пример: чек лог день"))
			return
		}
		period := strings.ToLower(parts[2])
		var duration time.Duration
		switch period {
		case "день":
			duration = 24 * time.Hour
		case "неделя":
			duration = 7 * 24 * time.Hour
		case "месяц":
			duration = 30 * 24 * time.Hour
		default:
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверный период. Используйте: день, неделя или месяц."))
			return
		}
		handleCheckLog(bot, message, duration)
	default:
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неизвестная админ команда."))
	}
}
