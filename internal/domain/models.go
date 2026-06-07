package domain

import (
	"errors"
	"time"
)

var WrongStatusCode = errors.New("wrong status code")

type QuestionStatus int

const (
	StatusUnknown QuestionStatus = iota
	StatusRed                    // Неправильно
	StatusYellow                 // Ответил, но есть нюансы. Надо повторить
	StatusGreen                  // Правильно
)

func (qs QuestionStatus) GetStatusByCode() (string, error) {
	switch qs {
	case StatusRed:
		return "[red]🔴", nil
	case StatusYellow:
		return "[yellow]🟡", nil
	case StatusGreen:
		return "[green]🟢", nil
	default:
		return "🤷‍♂️", WrongStatusCode
	}
}

func (qs QuestionStatus) ToInt() int {
	return int(qs)
}

type Question struct {
	ID        int64          `db:"id"`
	Question  string         `db:"question"`
	Answer    string         `db:"answer"`
	Status    QuestionStatus `db:"status"` // "[red]🔴", "[yellow]🟡", "[green]🟢"
	CreatedAt time.Time      `db:"created_at"`
	UpdatedAt time.Time      `db:"updated_at"`
}
