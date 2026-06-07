package repository

import (
	"errors"
	"interview/internal/domain"
)

var ErrNotFound = errors.New("entity not found")

// InterviewRepository описывает все действия с базой данных
type InterviewRepository interface {
	// GetQuestions Все вопросы
	GetQuestions(questions map[domain.QuestionStatus]map[int64]*domain.Question) (
		map[domain.QuestionStatus]map[int64]*domain.Question,
		error,
	)
	// AddQuestion Добавить вопрос
	AddQuestion(q *domain.Question) (*domain.Question, error)
	// UpdateQuestionStatus Изменить статус вопроса
	UpdateQuestionStatus(id int64, status int) error
	// RecountStatuses Обновить статусы вопросов
	RecountStatuses() error

	// История и настройки
	//AddToHistory(entry *HistoryEntry) error
	//GetTodayHistory() ([]HistoryEntry, error)
}
