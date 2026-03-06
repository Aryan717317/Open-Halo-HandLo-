package clipboard

import (
	"context"
	"log"

	"golang.design/x/clipboard"
)

// ClipboardSync manages system clipboard monitoring and updates.
type ClipboardSync struct {
	onChanged func(string)
}

func NewClipboardSync(onChanged func(string)) (*ClipboardSync, error) {
	err := clipboard.Init()
	if err != nil {
		return nil, err
	}
	return &ClipboardSync{onChanged: onChanged}, nil
}

// Start watching for system clipboard changes.
func (cs *ClipboardSync) Start(ctx context.Context) {
	ch := clipboard.Watch(ctx, clipboard.FmtText)
	log.Println("[Clipboard] Watcher started")
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-ch:
			text := string(data)
			if text != "" {
				cs.onChanged(text)
			}
		}
	}
}

// Write system clipboard with new text.
func (cs *ClipboardSync) Write(text string) {
	clipboard.Write(clipboard.FmtText, []byte(text))
}
