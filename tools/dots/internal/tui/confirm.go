package tui

import "context"

// Confirm требует явного yes; Enter, другой текст и отмена не подтверждают.
// Действие вызывающий выполняет только после nil.
func Confirm(ctx context.Context, frame Frame) error {
	frame.Prompt = "Введите yes"
	value, err := Input(ctx, frame, "")
	if err != nil {
		return err
	}
	if value != "yes" {
		return ErrCanceled
	}
	return nil
}
