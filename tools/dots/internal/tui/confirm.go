package tui

import "context"

// Confirm requires deliberate yes; Enter, other text and cancellation never approve.
// The caller owns all business rules and performs the action only after nil.
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
