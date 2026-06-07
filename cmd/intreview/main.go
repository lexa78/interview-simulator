package main

import (
	"database/sql"
	"fmt"
	"interview/internal/database"
	"interview/internal/domain"
	"interview/internal/repository/sqlite"
	"log/slog"
	"maps"
	"math/rand/v2"
	"os"
	"slices"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// updateStatusInMap Логика смены статуса
func updateStatusInMap(
	questions map[domain.QuestionStatus]map[int64]*domain.Question,
	currentStatus domain.QuestionStatus,
	questionId int64,
	statusForUpdate domain.QuestionStatus,
) bool {
	mapByStatus, ok := questions[currentStatus]
	if !ok {
		slog.Warn("Status not found in questions map", "status", currentStatus)
		return false
	}
	question, ok := mapByStatus[questionId]
	if !ok {
		slog.Warn("Question not found in questions map", "id", questionId)
		return false
	}
	question.Status = statusForUpdate

	return true
}

func createQuestionsMap() map[domain.QuestionStatus]map[int64]*domain.Question {
	result := make(map[domain.QuestionStatus]map[int64]*domain.Question)
	result[domain.StatusRed] = make(map[int64]*domain.Question)
	result[domain.StatusYellow] = make(map[int64]*domain.Question)
	result[domain.StatusGreen] = make(map[int64]*domain.Question)

	return result
}

type questionIdAndStatus struct {
	questionId int64
	status     domain.QuestionStatus
}

func main() {
	// todo сделать возможность передавать путь до файла с БД при старте приложения
	dbPath := "interviewer.db"

	db, err := database.InitDB(dbPath)
	if err != nil {
		slog.Error("Failed to connect to DB", "error", err)
		os.Exit(1)
	}
	defer func(db *sql.DB) {
		err = db.Close()
		if err != nil {
			slog.Error("Failed to close DB", "error", err)
		}
	}(db)

	repo := sqlite.NewRepositorySqlite(db)

	slog.Info("Migrations OK, DB is initialized")

	app := tview.NewApplication()
	app.EnableMouse(true) // Включаем поддержку мыши

	// Пересчитываем статусы в зависимости от прошлого ответа
	// По умолчанию неделя и больше - red, 4 дня до недели - yellow
	// todo сделать чтоб можно было передавать количества дней при запуске приложения
	err = repo.RecountStatuses()
	if err != nil {
		slog.Error("Failed to recount statuses", "error", err)
	}

	leftListMapIndexToQuestion := make(map[int]questionIdAndStatus)

	// Сюда будут выноситься вопросы, на которые сегдня дали ответ.
	// todo сделать кнопку, чтоб можно было повторить сегодняшние вопросы
	tmpQuestions := createQuestionsMap()

	questions := createQuestionsMap()
	questions, err = repo.GetQuestions(questions)
	if err != nil {
		slog.Error("Failed to get questions", "error", err)
		os.Exit(1)
	}

	// ЛЕВАЯ КОЛОНКА: Список вопросов
	leftList := tview.NewList().ShowSecondaryText(false)
	leftList.SetBorder(true).SetTitle(" Список вопросов ")

	// Функция обновления списка вопросов слева
	updateLeftList := func() {
		leftList.Clear()

		counter := 0
		for _, qst := range questions {
			for _, q := range qst {
				qStatus, err := q.Status.GetStatusByCode()
				if err != nil {
					slog.Warn("Status by code not found", "code", q.Status, "error", err)
				}
				itemText := fmt.Sprintf("%s %s", qStatus, q.Question)
				leftList.AddItem(itemText, "", 0, nil)
				leftListMapIndexToQuestion[counter] = questionIdAndStatus{
					questionId: q.ID,
					status:     q.Status,
				}
				counter++
			}
		}
	}
	updateLeftList()

	// ПРАВАЯ КОЛОНКА ВЕРХ: Прошедшие за сегодня
	rightHistory := tview.NewList().ShowSecondaryText(false)
	rightHistory.SetBorder(true).SetTitle(" Прошедшие за сегодня ")

	// ПРАВАЯ КОЛОНКА НИЗ: Рабочая зона (динамически меняется)
	workPages := tview.NewPages()
	workPages.SetBorder(true).SetTitle(" Тренажер ")

	// ГЛАВНЫЙ МАКЕТ (Разбиение экрана)
	mainFlex := tview.NewFlex().
		AddItem(leftList, 0, 1, true). // Левая колонка занимает 50%
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(rightHistory, 0, 1, false). // Верхняя правая часть
			AddItem(workPages, 0, 2, false),    // Нижняя правая часть (больше)
			0, 1, false)

	// ФУНКЦИЯ: Показ экрана вопроса
	var showQuestionPage func(q *domain.Question) // объявляем заранее для рекурсивного вызова

	showQuestionPage = func(q *domain.Question) {
		pageFlex := tview.NewFlex().SetDirection(tview.FlexRow)

		// Текст вопроса
		qWidget := tview.
			NewTextView().
			SetDynamicColors(true).
			SetText(fmt.Sprintf("[yellow]Вопрос:[white]\n%s", q.Question))
		pageFlex.AddItem(qWidget, 3, 1, false)

		// Поле ввода ответа
		inputField := tview.NewInputField().SetLabel("Ваш ответ: ").SetFieldWidth(0)
		pageFlex.AddItem(inputField, 3, 1, true)

		// Кнопка "Проверить"
		btnCheck := tview.NewButton("Проверить (Enter)").SetSelectedFunc(func() {
			// Очищаем рабочую зону для показа правильного ответа
			answerFlex := tview.NewFlex().SetDirection(tview.FlexRow)

			ansWidget := tview.NewTextView().SetDynamicColors(true).SetText(
				fmt.Sprintf(
					"[yellow]Вопрос:[white] %s\n\n[green]Правильный ответ:[white]\n%s",
					q.Question,
					q.Answer,
				),
			)
			answerFlex.AddItem(ansWidget, 0, 1, false)

			// Кнопки оценки
			btnFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

			btnRed := tview.NewButton("[red]Неправильно").SetSelectedFunc(func() {
				prevStatus := q.Status
				newStatus := domain.StatusRed
				if prevStatus != newStatus {
					err = repo.UpdateQuestionStatus(q.ID, newStatus.ToInt())
					if err != nil {
						slog.Error("Failed to update status", "error", err, "id", q.ID)
						return
					}

					isOk := updateStatusInMap(questions, prevStatus, q.ID, newStatus)
					if !isOk {
						slog.Error("Failed to update status", "status", q.Status, "id", q.ID)
					}
				}
				tmpQuestions[newStatus][q.ID] = q
				delete(questions[prevStatus], q.ID)

				rightHistory.AddItem(
					fmt.Sprintf("🔴 %s", q.Question),
					"",
					0,
					nil,
				)
				updateLeftList()
				workPages.SwitchToPage("menu")
				app.SetFocus(leftList)
			})

			btnYellow := tview.NewButton("[yellow]С правками").SetSelectedFunc(func() {
				prevStatus := q.Status
				newStatus := domain.StatusYellow
				if prevStatus != newStatus {
					err = repo.UpdateQuestionStatus(q.ID, newStatus.ToInt())
					if err != nil {
						slog.Error("Failed to update status", "error", err, "id", q.ID)
						return
					}

					isOk := updateStatusInMap(questions, prevStatus, q.ID, newStatus)
					if !isOk {
						slog.Error("Failed to update status", "status", q.Status, "id", q.ID)
					}
				}
				tmpQuestions[newStatus][q.ID] = q
				delete(questions[prevStatus], q.ID)

				rightHistory.AddItem(
					fmt.Sprintf("🟡 %s", q.Question),
					"",
					0,
					nil,
				)
				updateLeftList()
				workPages.SwitchToPage("menu")
				app.SetFocus(leftList)
			})

			btnGreen := tview.NewButton("[green]Правильно").SetSelectedFunc(func() {
				prevStatus := q.Status
				newStatus := domain.StatusGreen
				if prevStatus != newStatus {
					err = repo.UpdateQuestionStatus(q.ID, newStatus.ToInt())
					if err != nil {
						slog.Error("Failed to update status", "error", err, "id", q.ID)
						return
					}

					isOk := updateStatusInMap(questions, prevStatus, q.ID, newStatus)
					if !isOk {
						slog.Error("Failed to update status", "status", q.Status, "id", q.ID)
					}
				}
				tmpQuestions[newStatus][q.ID] = q
				delete(questions[prevStatus], q.ID)

				rightHistory.AddItem(
					fmt.Sprintf("🟢 %s", q.Question),
					"",
					0,
					nil,
				)
				updateLeftList()
				workPages.SwitchToPage("menu")
				app.SetFocus(leftList)
			})

			btnFlex.AddItem(btnRed, 0, 1, true).
				AddItem(btnYellow, 0, 1, false).
				AddItem(btnGreen, 0, 1, false)

			answerFlex.AddItem(btnFlex, 3, 1, true)
			workPages.AddPage("answer", answerFlex, true, true)
			workPages.SwitchToPage("answer")
			app.SetFocus(btnRed)
		})

		pageFlex.AddItem(btnCheck, 3, 1, false)

		workPages.AddPage("question", pageFlex, true, true)
		workPages.SwitchToPage("question")
		app.SetFocus(inputField)
	}

	// МЕНЮ УПРАВЛЕНИЯ (Стартовый экран в рабочей зоне)
	menuFlex := tview.NewFlex().SetDirection(tview.FlexRow)

	btnAuto := tview.NewButton("Автовопросы (Рандом)").SetSelectedFunc(func() {
		if len(questions[domain.StatusRed]) == 0 &&
			len(questions[domain.StatusYellow]) == 0 &&
			len(questions[domain.StatusGreen]) == 0 {
			return
		}
		// Сначала ищем красные, потом желтые, потом зеленые
		var (
			targetPool map[int64]*domain.Question
			ok         bool
		)
		for _, status := range []domain.QuestionStatus{
			domain.StatusRed,
			domain.StatusYellow,
			domain.StatusGreen,
		} {
			targetPool, ok = questions[status]
			if ok && len(targetPool) > 0 {
				break
			}
		}
		// Берем случайный из пула приоритетных
		keys := slices.Collect(maps.Keys(targetPool))
		randIndex := rand.IntN(len(keys))
		randomID := keys[randIndex]
		randomQ := targetPool[randomID]

		showQuestionPage(randomQ)
	})

	btnAdd := tview.NewButton("Добавить новый вопрос").SetSelectedFunc(func() {
		form := tview.NewForm().
			AddInputField("Вопрос", "", 0, nil, nil).
			AddTextArea("Ответ", "", 0, 5, 0, nil)

		form.AddButton("Сохранить", func() {
			qText := form.GetFormItem(0).(*tview.InputField).GetText()
			aText := form.GetFormItem(1).(*tview.TextArea).GetText()
			if qText != "" && aText != "" {
				rawQuestion := &domain.Question{
					Question: qText,
					Answer:   aText,
				}

				rawQuestion, err = repo.AddQuestion(rawQuestion)
				if err != nil {
					slog.Warn("Add question", "error", err)
					return
				}

				questions[domain.StatusRed][rawQuestion.ID] = rawQuestion
				updateLeftList()
			}
			workPages.SwitchToPage("menu")
			app.SetFocus(leftList)
		}).AddButton("Отмена", func() {
			workPages.SwitchToPage("menu")
			app.SetFocus(leftList)
		})

		workPages.AddPage("add", form, true, true)
		workPages.SwitchToPage("add")
		app.SetFocus(form)
	})

	menuFlex.
		AddItem(btnAuto, 3, 1, true).
		AddItem(btnAdd, 3, 1, false)
	workPages.AddPage("menu", menuFlex, true, true)

	// Клик по элементу в левом списке открывает этот вопрос вручную
	leftList.SetSelectedFunc(func(index int, name string, secondary string, shortcut rune) {
		showQuestionPage(questions[leftListMapIndexToQuestion[index].status][leftListMapIndexToQuestion[index].questionId])
	})

	// Глобальный обработчик кнопкой Escape для возврата в меню
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			workPages.SwitchToPage("menu")
			app.SetFocus(leftList)
			return nil
		}
		return event
	})

	// Запуск приложения
	if err = app.SetRoot(mainFlex, true).Run(); err != nil {
		slog.Error("App start error", "error", err)
		os.Exit(1)
	}
}
