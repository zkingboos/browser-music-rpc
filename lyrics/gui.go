package lyrics

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

var (
	fyneApp		fyne.App
	fyneWindow	fyne.Window
	songLabel	*widget.Label
	lyricLabel	*widget.Label
	StateUpdate	chan OverlayState
)

type OverlayState struct {
	SongName	string
	Current		string
}

func init() {
	StateUpdate = make(chan OverlayState, 100)
}

func ShowUI() {
	fyneApp = app.New()

	fyneWindow = fyneApp.NewWindow("Dynamic Island Lyrics")
	fyneWindow.SetPadded(false)
	fyneWindow.SetFixedSize(true)

	bg := canvas.NewRectangle(color.NRGBA{R: 20, G: 20, B: 25, A: 180})

	songData := binding.NewString()
	songData.Set("Waiting...")
	songLabel = widget.NewLabelWithData(songData)
	songLabel.Alignment = fyne.TextAlignCenter

	lyricData := binding.NewString()
	lyricData.Set("Connecting...")
	lyricLabel = widget.NewLabelWithData(lyricData)
	lyricLabel.Alignment = fyne.TextAlignCenter
	lyricLabel.TextStyle = fyne.TextStyle{Bold: true}

	content := container.NewStack(
		bg,
		container.New(layout.NewVBoxLayout(), songLabel, lyricLabel),
	)

	fyneWindow.SetContent(content)
	fyneWindow.Resize(fyne.NewSize(500, 100))

	go func() {
		for state := range StateUpdate {
			if state.Current == "" {
				state.Current = "🎵"
			}
			songData.Set(state.SongName)
			lyricData.Set(state.Current)
		}
	}()

	fyneWindow.ShowAndRun()
}

func Cleanup() {
	if fyneApp != nil {
		fyneApp.Quit()
	}
}

func broadcastOverlay(state OverlayState) {
	select {
	case StateUpdate <- state:
	default:
	}
}
