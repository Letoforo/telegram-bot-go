package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserProfile описывает анкету пользователя.
type UserProfile struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	TelegramID   int64              `bson:"telegram_id"`
	Username     string             `bson:"username"` // хранится в нижнем регистре
	Name         string             `bson:"name"`
	Race         string             `bson:"race"`
	Age          string             `bson:"age"`
	HeightWeight string             `bson:"height_weight"` // пример: "173.6 см\\70 кг"
	Gender       string             `bson:"gender"`
	PhotoFileID  string             `bson:"photo_file_id"`
	Rank         string             `bson:"rank"`      // по умолчанию "Ис"
	Team         string             `bson:"team"`      // по умолчанию "Наемник"
	Oblomki      int                `bson:"oblomki"`   // по умолчанию 0
	Piastry      int                `bson:"piastry"`   // по умолчанию 0
	Inventory    string             `bson:"inventory"` // по умолчанию "Пусто"
	IsAdmin      bool               `bson:"is_admin"`  // флаг администратора
}
