package lyrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type LyricLine struct {
	Time	float64
	Text	string
}

var (
	currentSong	string
	currentWsURL	string
	lyricsLines	[]LyricLine
	mu		sync.Mutex
	wsConn		*websocket.Conn
	reqID		int
	isRunning	bool
)

func getConn(wsURL string) *websocket.Conn {
	mu.Lock()
	defer mu.Unlock()

	if wsConn != nil && currentWsURL == wsURL {
		return wsConn
	}

	if wsConn != nil {
		wsConn.Close()
		wsConn = nil
	}

	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		wsConn = c
	}
	return c
}

func clearConn() {
	mu.Lock()
	defer mu.Unlock()
	if wsConn != nil {
		wsConn.Close()
		wsConn = nil
	}
}

func getCurrentTime(wsURL string) float64 {
	if wsURL == "" {
		return 0
	}

	c := getConn(wsURL)
	if c == nil {
		return 0
	}

	mu.Lock()
	reqID++
	id := reqID
	mu.Unlock()

	req := map[string]interface{}{
		"id":		id,
		"method":	"Runtime.evaluate",
		"params": map[string]interface{}{
			"expression": "(function() { let medias = document.querySelectorAll('video, audio'); let m = Array.from(medias).find(x => !x.paused) || medias[0]; return m ? m.currentTime : 0; })()",
		},
	}

	c.SetWriteDeadline(time.Now().Add(1 * time.Second))
	if err := c.WriteJSON(req); err != nil {
		clearConn()
		return -1
	}

	c.SetReadDeadline(time.Now().Add(1 * time.Second))
	for {
		var resp struct {
			ID	int	`json:"id"`
			Result	struct {
				Result struct {
					Value float64 `json:"value"`
				} `json:"result"`
			}	`json:"result"`
		}
		if err := c.ReadJSON(&resp); err != nil {
			clearConn()
			return -1
		}

		if resp.ID == id {
			return resp.Result.Result.Value
		}
	}
}

func fetchLyrics(songName string) []LyricLine {
	cleanName := strings.ReplaceAll(songName, " - YouTube", "")
	cleanName = strings.ReplaceAll(cleanName, " | YouTube Music", "")

	var apiURL string
	if strings.Contains(cleanName, " - ") {
		parts := strings.SplitN(cleanName, " - ", 2)
		artist := strings.TrimSpace(parts[0])
		track := strings.TrimSpace(parts[1])

		if idx := strings.Index(track, "("); idx > 0 {
			track = strings.TrimSpace(track[:idx])
		}
		if idx := strings.Index(track, "["); idx > 0 {
			track = strings.TrimSpace(track[:idx])
		}

		apiURL = fmt.Sprintf("https://lrclib.net/api/search?artist_name=%s&track_name=%s", url.QueryEscape(artist), url.QueryEscape(track))
	} else {

		if idx := strings.Index(cleanName, "("); idx > 0 {
			cleanName = strings.TrimSpace(cleanName[:idx])
		}
		if idx := strings.Index(cleanName, "["); idx > 0 {
			cleanName = strings.TrimSpace(cleanName[:idx])
		}
		apiURL = fmt.Sprintf("https://lrclib.net/api/search?q=%s", url.QueryEscape(cleanName))
	}

	resp, err := http.Get(apiURL)
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()

	var result []struct {
		SyncedLyrics string `json:"syncedLyrics"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	for _, res := range result {
		if res.SyncedLyrics != "" {
			lines := parseLRC(res.SyncedLyrics)
			if len(lines) > 0 {
				return lines
			}
		}
	}

	return nil
}

func parseLRC(lrc string) []LyricLine {
	var lines []LyricLine
	for _, line := range strings.Split(lrc, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 10 || !strings.HasPrefix(line, "[") || line[9] != ']' {
			continue
		}

		minStr := line[1:3]
		secStr := line[4:6]
		msStr := line[7:9]

		min, err1 := strconv.ParseFloat(minStr, 64)
		sec, err2 := strconv.ParseFloat(secStr, 64)
		ms, err3 := strconv.ParseFloat(msStr, 64)

		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}

		timeInSeconds := (min * 60) + sec + (ms / 100)
		text := strings.TrimSpace(line[10:])

		lines = append(lines, LyricLine{
			Time:	timeInSeconds,
			Text:	text,
		})
	}
	return lines
}

func StartOrUpdate(songName, wsURL string) {
	mu.Lock()
	defer mu.Unlock()

	if currentSong == songName {
		currentWsURL = wsURL
		return
	}

	currentSong = songName
	currentWsURL = wsURL
	lyricsLines = nil

	if !isRunning {
		isRunning = true
		go renderLoop()
	}

	go func(song string) {
		lines := fetchLyrics(song)
		mu.Lock()
		defer mu.Unlock()
		if currentSong == song {
			if len(lines) == 0 {
				lines = []LyricLine{
					{Time: 0, Text: "(Letra sincronizada não encontrada)"},
				}
			}
			lyricsLines = lines
		}
	}(songName)
}

func renderLoop() {
	var lastTime float64
	for {
		mu.Lock()
		song := currentSong
		wsURL := currentWsURL
		lines := lyricsLines
		mu.Unlock()

		if song == "" || wsURL == "" {
			time.Sleep(1 * time.Second)
			continue
		}

		var sb strings.Builder
		sb.WriteString("\033[H\033[2J")

		var state OverlayState
		state.SongName = song

		if len(lines) == 0 {
			sb.WriteString(fmt.Sprintf("\033[36m=== Tocando Agora: %s ===\033[0m\n\n", song))
			sb.WriteString("\033[90mBuscando letra...\033[0m\n")
			fmt.Print(sb.String())
			broadcastOverlay(state)
			time.Sleep(1 * time.Second)
			continue
		}

		t := getCurrentTime(wsURL)
		if t >= 0 {
			lastTime = t
		} else {
			lastTime += 0.05
		}
		currentTime := lastTime

		mins := int(currentTime) / 60
		secs := int(currentTime) % 60
		sb.WriteString(fmt.Sprintf("\033[36m=== Tocando Agora: %s [%02d:%02d] ===\033[0m\n\n", song, mins, secs))

		var currentIndex int = -1
		for i, line := range lines {
			if currentTime >= line.Time {
				currentIndex = i
			} else {
				break
			}
		}

		if currentIndex == -1 {
			sb.WriteString("\033[90mAguardando o trecho inicial...\033[0m\n")
			state.Current = "Aguardando o trecho inicial..."
		} else {

			start := currentIndex - 3
			if start < 0 {
				start = 0
			}
			for i := start; i < currentIndex; i++ {
				sb.WriteString(fmt.Sprintf("\033[90m%s\033[0m\n", lines[i].Text))
			}

			sb.WriteString(fmt.Sprintf("\033[1;32m%s\033[0m\n", lines[currentIndex].Text))
			state.Current = lines[currentIndex].Text

			end := currentIndex + 4
			if end > len(lines) {
				end = len(lines)
			}
			for i := currentIndex + 1; i < end; i++ {
				sb.WriteString(fmt.Sprintf("\033[37m%s\033[0m\n", lines[i].Text))
			}
		}

		fmt.Print(sb.String())
		broadcastOverlay(state)

		time.Sleep(50 * time.Millisecond)
	}
}
