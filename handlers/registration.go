package handlers

import (
	"strings"

	"telegram-bot-go/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type RegistrationSession struct {
	Step int
	Data models.UserProfile
}

// Глобальное хранилище сеансов регистрации.
var registrationSessions = make(map[int64]*RegistrationSession)

// StartRegistration начинает процесс регистрации, запрашивая имя/псевдоним.
func StartRegistration(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	registrationSessions[message.From.ID] = &RegistrationSession{
		Step: 1,
		Data: models.UserProfile{
			TelegramID: message.From.ID,
			Username:   strings.ToLower(message.From.UserName),
			Rank:       "Ис",
			Team:       "Наемник",
			Oblomki:    0,
			Piastry:    0,
			Inventory:  "Пусто",
		},
	}
	reply := "Введите имя и/или псевдоним:"
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, reply))
}

// ProcessRegistrationStep обрабатывает шаги регистрации.
func ProcessRegistrationStep(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	session, exists := registrationSessions[message.From.ID]
	if !exists {
		return
	}
	// Если на шаге 6 ожидается фотография
	if len(message.Photo) > 0 && session.Step == 6 {
		photo := message.Photo[len(message.Photo)-1]
		session.Data.PhotoFileID = photo.FileID
		SaveUserProfile(session.Data)
		reply := "Добро пожаловать на борт!"
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, reply))
		delete(registrationSessions, message.From.ID)
		return
	}
	switch session.Step {
	case 1:
		session.Data.Name = strings.TrimSpace(message.Text)
		// Если регистрируется постоянный админ, добавляем эмодзи.
		if strings.EqualFold(message.From.UserName, "My_Beautifu1_Madness") {
			adminEmoji := "🏴‍☠️"
			if !strings.Contains(session.Data.Name, adminEmoji) {
				session.Data.Name = session.Data.Name + " " + adminEmoji
			}
			session.Data.IsAdmin = true
		}
		session.Step++
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Введите расу:"))
	case 2:
		session.Data.Race = strings.TrimSpace(message.Text)
		session.Step++
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Введите возраст:"))
	case 3:
		session.Data.Age = strings.TrimSpace(message.Text)
		session.Step++
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Введите рост и вес (например: 173.6 см\\70 кг):"))
	case 4:
		session.Data.HeightWeight = strings.TrimSpace(message.Text)
		session.Step++
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Введите пол:"))
	case 5:
		session.Data.Gender = strings.TrimSpace(message.Text)
		session.Step++
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Отправьте фотографию персонажа:"))
	default:
		// Ничего не делаем для неизвестного шага.
	}
}

// resetRegistrationSessions очищает все активные сеансы регистрации.
func resetRegistrationSessions() {
	for id := range registrationSessions {
		delete(registrationSessions, id)
	}
}
