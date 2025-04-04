package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	// Импорт необходимых пакетов MongoDB.
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"telegram-bot-go/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var userCollection *mongo.Collection
var logsCollection *mongo.Collection

// InitHandlers инициализирует коллекции пользователей и логов.
func InitHandlers(db *mongo.Database) {
	userCollection = db.Collection("users")
	logsCollection = db.Collection("logs")

	// Создаем TTL-индекс для логов (удаление документов старше 30 дней).
	indexModel := mongo.IndexModel{
		Keys:    bson.M{"date": 1},
		Options: options.Index().SetExpireAfterSeconds(2592000),
	}
	_, err := logsCollection.Indexes().CreateOne(context.Background(), indexModel)
	if err != nil {
		log.Printf("Ошибка создания TTL индекса для логов: %v", err)
	}
}

// AddLogEvent записывает событие изменения ресурса (при добавлении или передаче) в коллекцию логов.
func AddLogEvent(userProfile models.UserProfile, changeAmount int, resource string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	event := bson.M{
		"date":          time.Now(),
		"telegram_id":   userProfile.TelegramID,
		"username":      userProfile.Username,
		"name":          userProfile.Name,
		"change_amount": changeAmount,
		"resource":      resource,
	}
	_, err := logsCollection.InsertOne(ctx, event)
	return err
}

// HandleCommand обрабатывает стандартные команды для обычных пользователей.
func HandleCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	cmd := strings.ToLower(strings.TrimSpace(message.Text))
	switch cmd {
	case "где ром":
		resetRegistrationSessions()
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Все выпили, Капитан!"))
	case "регистрация":
		StartRegistration(bot, message)
	case "анкета":
		showUserProfile(bot, message)
	case "статистика":
		handleStatistic(bot, message)
	case "удалить анкету":
		handleDeleteProfile(bot, message)
	// команды помощи:
	case "помоги", "помощь", "я забыл", "забыл", "список команд", "что ты умеешь", "что ты делаешь":
		handleHelp(bot, message)
	default:
		parts := strings.Fields(cmd)
		if len(parts) >= 2 {
			switch parts[0] {
			case "изменить":
				if len(parts) < 3 {
					bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверный формат. Например: изменить имя НовоеИмя"))
					return
				}
				field := parts[1]
				newValue := strings.Join(parts[2:], " ")
				changeUserProfileField(bot, message, field, newValue)
			case "добавить":
				if len(parts) < 3 {
					bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверный формат. Например: добавить обломки 5"))
					return
				}
				handleAdd(bot, message, parts[1], parts[2])
			case "потерять":
				if len(parts) < 2 {
					bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверный формат команды."))
					return
				}
				handleShow(bot, message, parts[1])
			case "передать":
				// Формат: передать (обломки или пиастры) (@username) (количество)
				if len(parts) < 4 {
					bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверный формат. Пример: передать обломки @username 5"))
					return
				}
				handleTransfer(bot, message, parts[1], parts[2], parts[3])
			}
		}
	}
}

// HandleNonCommandMessage обрабатывает некомандные сообщения (например, шаги регистрации).
func HandleNonCommandMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	if _, ok := registrationSessions[message.From.ID]; ok {
		ProcessRegistrationStep(bot, message)
	}
}

// SaveUserProfile сохраняет или обновляет анкету пользователя в базе.
func SaveUserProfile(profile models.UserProfile) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"telegram_id": profile.TelegramID}
	var existing models.UserProfile
	err := userCollection.FindOne(ctx, filter).Decode(&existing)
	if err == nil {
		update := bson.M{"$set": profile}
		userCollection.UpdateOne(ctx, filter, update)
	} else {
		userCollection.InsertOne(ctx, profile)
	}
}

// showUserProfile извлекает анкету пользователя из базы и отправляет её.
func showUserProfile(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"telegram_id": message.From.ID}
	var profile models.UserProfile
	err := userCollection.FindOne(ctx, filter).Decode(&profile)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Анкета не найдена. Зарегистрируйтесь командой: регистрация"))
		return
	}
	caption := fmt.Sprintf(
		"Имя: %s\nРаса: %s\nВозраст: %s\nРост и вес: %s\nПол: %s\nРанг: %s\nКоманда: %s\nОбломки: %d\nПиастры: %d\nИнвентарь: %s",
		profile.Name, profile.Race, profile.Age, profile.HeightWeight,
		profile.Gender, profile.Rank, profile.Team, profile.Oblomki,
		profile.Piastry, profile.Inventory)
	photoMsg := tgbotapi.NewPhoto(message.Chat.ID, tgbotapi.FileID(profile.PhotoFileID))
	photoMsg.Caption = caption
	bot.Send(photoMsg)
}

// changeUserProfileField изменяет указанное поле анкеты.
func changeUserProfileField(bot *tgbotapi.BotAPI, message *tgbotapi.Message, field, newValue string) {
	allowedFields := map[string]string{
		"имя":       "name",
		"раса":      "race",
		"возраст":   "age",
		"ростивес":  "height_weight",
		"пол":       "gender",
		"ранг":      "rank",
		"команда":   "team",
		"инвентарь": "inventory",
	}
	dbField, ok := allowedFields[field]
	if !ok {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Поле для изменения не поддерживается."))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"telegram_id": message.From.ID}
	update := bson.M{"$set": bson.M{dbField: newValue}}
	_, err := userCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка при изменении профиля."))
		return
	}
	reply := fmt.Sprintf("Поле '%s' успешно изменено.", field)
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, reply))
}

// handleAdd обрабатывает команды вида "добавить обломки 5" или "добавить пиастры 5".
func handleAdd(bot *tgbotapi.BotAPI, message *tgbotapi.Message, field, valueStr string) {
	num, err := strconv.Atoi(strings.TrimSpace(valueStr))
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверное значение количества."))
		return
	}
	var dbField string
	if strings.ToLower(field) == "обломки" {
		dbField = "oblomki"
	} else if strings.ToLower(field) == "пиастры" {
		dbField = "piastry"
	} else {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверное поле. Используйте 'обломки' или 'пиастры'."))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"telegram_id": message.From.ID}
	update := bson.M{"$inc": bson.M{dbField: num}}
	_, err = userCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка при обновлении профиля."))
		return
	}
	reply := fmt.Sprintf("Добавлено %d к %s.", num, field)
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, reply))
	// Запись лога
	var currentUser models.UserProfile
	if err := userCollection.FindOne(ctx, filter).Decode(&currentUser); err == nil {
		AddLogEvent(currentUser, num, field)
	}
}

// handleShow выводит текущее значение ресурса.
func handleShow(bot *tgbotapi.BotAPI, message *tgbotapi.Message, field string) {
	lField := strings.ToLower(field)
	if lField != "обломки" && lField != "пиастры" {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверное поле. Используйте 'обломки' или 'пиастры'."))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"telegram_id": message.From.ID}
	var profile models.UserProfile
	err := userCollection.FindOne(ctx, filter).Decode(&profile)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Анкета не найдена."))
		return
	}
	var currentValue int
	if lField == "обломки" {
		currentValue = profile.Oblomki
	} else {
		currentValue = profile.Piastry
	}
	reply := fmt.Sprintf("Текущее значение %s: %d", field, currentValue)
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, reply))
}

// handleTransfer осуществляет передачу ресурса от отправителя к получателю.
// Формат команды: передать (обломки или пиастры) (@username) (количество)
func handleTransfer(bot *tgbotapi.BotAPI, message *tgbotapi.Message, field, targetUser, amountStr string) {
	amount, err := strconv.Atoi(amountStr)
	if err != nil || amount <= 0 {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверное значение количества для передачи."))
		return
	}
	var dbField string
	switch strings.ToLower(field) {
	case "обломки":
		dbField = "oblomki"
	case "пиастры":
		dbField = "piastry"
	default:
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Неверное поле. Используйте 'обломки' или 'пиастры'."))
		return
	}
	targetUsername := strings.TrimPrefix(targetUser, "@")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Получаем профиль отправителя.
	donorFilter := bson.M{"telegram_id": message.From.ID}
	var donor models.UserProfile
	err = userCollection.FindOne(ctx, donorFilter).Decode(&donor)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ваша анкета не найдена."))
		return
	}
	var donorAmount int
	if dbField == "oblomki" {
		donorAmount = donor.Oblomki
	} else {
		donorAmount = donor.Piastry
	}
	if donorAmount < amount {
		reply := fmt.Sprintf("У вас недостаточно %s для передачи.", field)
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, reply))
		return
	}
	// Получаем профиль получателя (username хранится в нижнем регистре).
	recipientFilter := bson.M{"username": strings.ToLower(targetUsername)}
	var recipient models.UserProfile
	err = userCollection.FindOne(ctx, recipientFilter).Decode(&recipient)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Профиль получателя не найден. Убедитесь, что пользователь зарегистрирован."))
		return
	}
	// Обновляем профиль отправителя: списываем ресурс.
	donorUpdate := bson.M{"$inc": bson.M{dbField: -amount}}
	_, err = userCollection.UpdateOne(ctx, donorFilter, donorUpdate)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка при списании средств с вашего баланса."))
		return
	}
	// Обновляем профиль получателя: прибавляем ресурс.
	recipientUpdate := bson.M{"$inc": bson.M{dbField: amount}}
	_, err = userCollection.UpdateOne(ctx, recipientFilter, recipientUpdate)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Ошибка при зачислении средств получателю."))
		return
	}
	reply := fmt.Sprintf("Передача выполнена успешно. Вы передали %d %s пользователю @%s.", amount, field, targetUsername)
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, reply))
	// Записываем логи для отправителя и получателя.
	AddLogEvent(donor, -amount, field)
	AddLogEvent(recipient, amount, field)
}

// handleStatistic открывает инлайн-клавиатуру для выбора варианта статистики.
func handleStatistic(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Пиастры", "stat:piastry"),
			tgbotapi.NewInlineKeyboardButtonData("Обломки", "stat:oblomki"),
			tgbotapi.NewInlineKeyboardButtonData("Оба", "stat:both"),
		),
	)
	msg := tgbotapi.NewMessage(message.Chat.ID, "Выберите вариант статистики:")
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// handleHelp выводит список команд для пользователя.
func handleHelp(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	helpText := "Команды для обычных пользователей:\n" +
		"• регистрация – начать регистрацию анкеты\n" +
		"• анкета – показать свою анкету\n" +
		"• где ром – сбросить незавершённую регистрацию\n" +
		"• статистика – показать статистику участников\n" +
		"• изменить [поле] [значение] – изменить указанное поле анкеты\n" +
		"• добавить [обломки/пиастры] [количество] – пополнить ресурс\n" +
		"• потерять [обломки/пиастры] – увидеть текущее значение ресурса\n" +
		"• передать [обломки/пиастры] (@username) [количество] – передать ресурс другому участнику\n" +
		"• удалить анкету – удалить свою анкету (требуется подтверждение)\n\n" +
		"Команды для администрации:\n" +
		"• список анкет – вывести краткий список анкет всех участников\n" +
		"• полный список анкет – вывести каждую анкету с подробностями и фотографией\n" +
		"• анкета (айди анкеты) – вывести анкету по заданному ID\n" +
		"• датьадмин @username – назначить пользователя администратором\n" +
		"• живой – сбросить все активные сеансы регистрации\n" +
		"• чек лог [день/неделя/месяц] – вывести лог изменений ресурсов\n"
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, helpText))
}

// handleDeleteProfile отправляет сообщение с инлайн-клавиатурой для подтверждения удаления анкеты.
func handleDeleteProfile(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	yesButton := tgbotapi.NewInlineKeyboardButtonData("Да", "deleteprofile:yes")
	noButton := tgbotapi.NewInlineKeyboardButtonData("Нет", "deleteprofile:no")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(yesButton, noButton))
	msg := tgbotapi.NewMessage(message.Chat.ID, "Вы точно хотите удалить анкету?")
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// HandleCreateEvent обрабатывает команду создания ивента от администратора.
// Формат команды:
//
//	начатьивент Название ивента, число для обломков, число для пиастр
func HandleCreateEvent(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	// Удаляем префикс и обрезаем пробелы
	argsStr := strings.TrimSpace(strings.TrimPrefix(message.Text, "начатьивент"))
	parts := strings.Split(argsStr, ",")
	if len(parts) < 3 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Неверный формат команды.\nИспользуйте: начатьивент (Имя ивента), (число для обломков), (число для пиастр)")
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

	// Создаём объект активного ивента
	currentEvent = &EventDetails{
		Name:      eventName,
		Oblomki:   oblomki,
		Piastry:   piastry,
		StartDate: time.Now(),
	}

	eventMessage := fmt.Sprintf("Ивент %s Запущен!\nУчастникам, принявшим ивент, будет зачислено:\nОбломков: %d\nПиастр: %d\nДата начала: %s",
		currentEvent.Name, currentEvent.Oblomki, currentEvent.Piastry, currentEvent.StartDate.Format("02.01.2006 15:04"))

	// Создаём инлайн-клавиатуру с двумя кнопками.
	participateButton := tgbotapi.NewInlineKeyboardButtonData("Участвую", "event:participate")
	skipButton := tgbotapi.NewInlineKeyboardButtonData("Пропуск", "event:skip")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(participateButton, skipButton))

	msg := tgbotapi.NewMessage(message.Chat.ID, eventMessage)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// HandleCallbackQuery обрабатывает callback-запросы (например, для статистики и подтверждения удаления анкеты).
func HandleCallbackQuery(bot *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
	// Отвечаем на callback-запрос, чтобы кнопки перестали мигать.
	ack := tgbotapi.NewCallback(cq.ID, "")
	bot.Request(ack)

	// Если это ответ на удаление анкеты.
	if strings.HasPrefix(cq.Data, "deleteprofile:") {
		switch cq.Data {
		case "deleteprofile:yes":
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			filter := bson.M{"telegram_id": cq.From.ID}
			res, err := userCollection.DeleteOne(ctx, filter)
			if err != nil || res.DeletedCount == 0 {
				bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Ошибка при удалении анкеты."))
			} else {
				bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Анкета удалена."))
			}
		case "deleteprofile:no":
			bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Удаление отменено."))
		}
		return
	}

	// Обработка callback-запроса для статистики.
	var sortOptions *options.FindOptions
	var header, statType string
	switch cq.Data {
	case "stat:piastry":
		sortOptions = options.Find().SetSort(bson.D{{Key: "piastry", Value: -1}})
		header = "Имя | Ранг | Команда | Пиастры\n"
		statType = "piastry"
	case "stat:oblomki":
		sortOptions = options.Find().SetSort(bson.D{{Key: "oblomki", Value: -1}})
		header = "Имя | Ранг | Команда | Обломки\n"
		statType = "oblomki"
	case "stat:both":
		sortOptions = options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
		header = "Имя | Ранг | Команда | Обломки | Пиастры\n"
		statType = "both"
	default:
		bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Неверный выбор статистики."))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cursor, err := userCollection.Find(ctx, bson.M{}, sortOptions)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, "Ошибка при получении статистики."))
		return
	}
	defer cursor.Close(ctx)

	var result strings.Builder
	result.WriteString(header)
	for cursor.Next(ctx) {
		var profile models.UserProfile
		if err := cursor.Decode(&profile); err != nil {
			continue
		}
		switch statType {
		case "piastry":
			result.WriteString(fmt.Sprintf("%s | %s | %s | %d\n", profile.Name, profile.Rank, profile.Team, profile.Piastry))
		case "oblomki":
			result.WriteString(fmt.Sprintf("%s | %s | %s | %d\n", profile.Name, profile.Rank, profile.Team, profile.Oblomki))
		case "both":
			result.WriteString(fmt.Sprintf("%s | %s | %s | %d | %d\n", profile.Name, profile.Rank, profile.Team, profile.Oblomki, profile.Piastry))
		}
	}
	if strings.TrimSpace(result.String()) == header {
		result.WriteString("Нет данных для отображения.")
	}
	bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, result.String()))
}
