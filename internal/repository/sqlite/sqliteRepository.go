package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"interview/internal/domain"
	"interview/internal/repository"
	"log/slog"
	"time"
)

type RepositorySqlite struct {
	db *sql.DB
}

func NewRepositorySqlite(db *sql.DB) *RepositorySqlite {
	db.SetMaxOpenConns(1)
	return &RepositorySqlite{db: db}
}

func (r RepositorySqlite) GetQuestions(
	questions map[domain.QuestionStatus]map[int64]*domain.Question,
) (
	map[domain.QuestionStatus]map[int64]*domain.Question,
	error,
) {
	query := `SELECT id, question, answer, status
				FROM questions
				GROUP BY status, id
				ORDER BY status;`
	rows, err := r.db.Query(query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}

		return nil, fmt.Errorf("get questions: %w", err)
	}
	defer func(rows *sql.Rows) {
		err = rows.Close()
		if err != nil {
			slog.Warn("failed to close rows: ", "error", err)
		}
	}(rows)

	for rows.Next() {
		var (
			question    = &domain.Question{}
			statusAsInt int
		)
		err = rows.Scan(
			&question.ID,
			&question.Question,
			&question.Answer,
			&statusAsInt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan questions: %w", err)
		}
		question.Status = domain.QuestionStatus(statusAsInt)
		questions[question.Status][question.ID] = question
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows next: %w", err)
	}

	return questions, nil
}

func (r RepositorySqlite) AddQuestion(q *domain.Question) (*domain.Question, error) {
	query := `INSERT INTO questions (question, answer, updated_at)
				VALUES (?, ?, datetime('now')) RETURNING id, status;`
	var statusAsInt int
	err := r.db.QueryRow(query, q.Question, q.Answer).Scan(&q.ID, &statusAsInt)
	if err != nil {
		return nil, fmt.Errorf("insert question: %w", err)
	}

	q.Status = domain.QuestionStatus(statusAsInt)

	return q, nil
}

func (r RepositorySqlite) UpdateQuestionStatus(id int64, status int) error {
	query := `UPDATE questions SET status = ?, updated_at = datetime('now') WHERE id = ?;`
	result, err := r.db.Exec(query, status, id)
	if err != nil {
		return fmt.Errorf("update question status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update question status: %w", err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r RepositorySqlite) RecountStatuses() error {
	query := `
	UPDATE questions
	SET status = CASE 
		WHEN updated_at < ? THEN ?
		WHEN updated_at < ? THEN ?
	END
	WHERE 
		(updated_at < ? AND status IN (?, ?))
		OR 
		(updated_at >= ? AND updated_at < ? AND status = ?);`

	// Высчитываем временные границы на стороне Go (так надежнее для часовых поясов)
	now := time.Now().UTC()
	fourDaysAgo := now.AddDate(0, 0, -4)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	// Передаем аргументы по порядку их появления в запросе
	_, err := r.db.Exec(query,
		sevenDaysAgo, int(domain.StatusRed), fourDaysAgo, int(domain.StatusYellow), // SET clause
		sevenDaysAgo, int(domain.StatusYellow), int(domain.StatusGreen), // WHERE clause (red check)
		sevenDaysAgo, fourDaysAgo, int(domain.StatusGreen), // WHERE clause (yellow check)
	)

	if err != nil {
		return fmt.Errorf("recount statuses: %w", err)
	}

	return nil
}
