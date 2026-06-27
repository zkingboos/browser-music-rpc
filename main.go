package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"brave-scraper/lyrics"

	"github.com/gorilla/websocket"
	"github.com/hugolgst/rich-go/client"
)

type Target struct {
	ID			string	`json:"id"`
	Title			string	`json:"title"`
	URL			string	`json:"url"`
	Type			string	`json:"type"`
	WebSocketDebuggerURL	string	`json:"webSocketDebuggerUrl"`
}

var artworkCache = make(map[string]string)

func getArtwork(songName string) string {
	if img, ok := artworkCache[songName]; ok {
		return img
	}

	cleanName := strings.ReplaceAll(songName, " - YouTube", "")
	cleanName = strings.ReplaceAll(cleanName, " | YouTube Music", "")

	searchQuery := url.QueryEscape(cleanName)
	apiURL := fmt.Sprintf("https://itunes.apple.com/search?term=%s&media=music&limit=1", searchQuery)

	resp, err := http.Get(apiURL)
	if err != nil || resp.StatusCode != 200 {
		return "https://raw.githubusercontent.com/hugolgst/rich-go/master/assets/large.png"
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ArtworkUrl100 string `json:"artworkUrl100"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "https://raw.githubusercontent.com/hugolgst/rich-go/master/assets/large.png"
	}

	if len(result.Results) > 0 {

		bigImg := strings.Replace(result.Results[0].ArtworkUrl100, "100x100bb", "512x512bb", 1)
		artworkCache[songName] = bigImg
		return bigImg
	}

	defaultIcon := "https://raw.githubusercontent.com/hugolgst/rich-go/master/assets/large.png"
	artworkCache[songName] = defaultIcon
	return defaultIcon
}

func extractYoutubeThumbnail(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	videoID := u.Query().Get("v")
	if videoID != "" {

		return fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", videoID)
	}
	return ""
}

func getMediaInfo(wsURL string) (isPlaying bool, currentTime float64, docTitle string, channel string) {
	if wsURL == "" {
		return false, 0, "", ""
	}
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return false, 0, "", ""
	}
	defer c.Close()

	req := map[string]interface{}{
		"id":		1,
		"method":	"Runtime.evaluate",
		"params": map[string]interface{}{
			"expression": `(function() { 
				let medias = document.querySelectorAll('video, audio'); 
				let m = Array.from(medias).find(x => !x.paused) || medias[0];
				let isPlaying = m ? !m.paused : false;
				let currentTime = m ? m.currentTime : 0;
				let titleEl = document.querySelector('yt-formatted-string.title.ytmusic-player-bar') || document.querySelector('h1.ytd-watch-metadata yt-formatted-string') || document.querySelector('h1.title yt-formatted-string');
				let domTitle = titleEl ? titleEl.innerText.trim() : '';
				let docTitle = domTitle || document.title || '';
				let el = document.querySelector('yt-formatted-string.byline.ytmusic-player-bar a') || document.querySelector('.byline.ytmusic-player-bar a') || document.querySelector('#owner-name a') || document.querySelector('.ytd-channel-name a') || document.querySelector('ytd-channel-name yt-formatted-string');
				let channel = el ? el.innerText.trim() : '';
				return JSON.stringify({isPlaying, currentTime, docTitle, channel});
			})()`,
		},
	}

	c.SetWriteDeadline(time.Now().Add(1 * time.Second))
	if err := c.WriteJSON(req); err != nil {
		return false, 0, "", ""
	}

	var resp struct {
		Result struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		} `json:"result"`
	}

	c.SetReadDeadline(time.Now().Add(1 * time.Second))
	if err := c.ReadJSON(&resp); err != nil {
		return false, 0, "", ""
	}

	var result struct {
		IsPlaying	bool	`json:"isPlaying"`
		CurrentTime	float64	`json:"currentTime"`
		DocTitle	string	`json:"docTitle"`
		Channel		string	`json:"channel"`
	}
	json.Unmarshal([]byte(resp.Result.Result.Value), &result)

	return result.IsPlaying, result.CurrentTime, result.DocTitle, result.Channel
}

func main() {

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nEncerrando o programa e fechando a janela flutuante...")
		lyrics.Cleanup()
		os.Exit(0)
	}()

	discordClientID := os.Getenv("DISCORD_CLIENT_ID")
	if discordClientID == "" {
		log.Println("Aviso: A variável de ambiente DISCORD_CLIENT_ID não está definida.")
	}

	err := client.Login(discordClientID)
	if err != nil {
		log.Printf("Erro ao conectar no Discord: %v\n", err)
	} else {
		fmt.Println("Conectado ao Discord Rich Presence!")
		defer client.Logout()
	}

	fmt.Println("Iniciando monitoramento de abas e tempo de música...")

	var currentPlaying string
	var songStartTime time.Time

	go func() {
		for {
			resp, err := http.Get("http://127.0.0.1:9222/json/list")
			if err != nil {
				log.Printf("Aguardando navegador... (Erro: %v)\n", err)
				time.Sleep(5 * time.Second)
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			var targets []Target
			if err := json.Unmarshal(body, &targets); err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			var songState string
			var statusDetails string
			var currentImage string
			var wsURL string
			foundMusic := false

			var activeWsURL string
			var finalCurrentTime float64

			for _, t := range targets {
				if t.Type != "page" {
					continue
				}

				pageURL := t.URL

				if strings.Contains(pageURL, "youtube.com") {
					wsURL = t.WebSocketDebuggerURL

					playing, cTime, docTitle, channel := getMediaInfo(wsURL)

					if !playing || docTitle == "" {
						continue
					}

					pageTitle := docTitle

					cleanTitle := strings.ReplaceAll(pageTitle, " - YouTube", "")
					cleanTitle = strings.ReplaceAll(cleanTitle, " | YouTube Music", "")

					if strings.HasPrefix(cleanTitle, "(") && strings.Contains(cleanTitle, ") ") {
						idx := strings.Index(cleanTitle, ") ")
						if idx != -1 {
							cleanTitle = cleanTitle[idx+2:]
						}
					}

					if cleanTitle == "YouTube" || cleanTitle == "YouTube Music" {
						continue
					}

					if !strings.Contains(cleanTitle, " - ") {
						if channel != "" {
							cleanTitle = channel + " - " + cleanTitle
						}
					}

					statusDetails = "🎵 Ouvindo no YouTube"
					songState = cleanTitle
					currentImage = getArtwork(songState)

					activeWsURL = wsURL
					finalCurrentTime = cTime
					foundMusic = true
					break
				} else if strings.Contains(pageURL, "open.spotify.com") {
					playing, cTime, docTitle, _ := getMediaInfo(t.WebSocketDebuggerURL)
					if !playing {
						continue
					}
					statusDetails = "🎵 Ouvindo no Spotify"
					songState = strings.ReplaceAll(docTitle, " - Spotify", "")
					currentImage = getArtwork(songState)
					activeWsURL = t.WebSocketDebuggerURL
					finalCurrentTime = cTime
					foundMusic = true
					break
				} else if strings.Contains(pageURL, "soundcloud.com") {
					playing, cTime, docTitle, _ := getMediaInfo(t.WebSocketDebuggerURL)
					if !playing {
						continue
					}
					statusDetails = "🎵 Ouvindo no SoundCloud"
					songState = docTitle
					currentImage = getArtwork(songState)
					activeWsURL = t.WebSocketDebuggerURL
					finalCurrentTime = cTime
					foundMusic = true
					break
				}
			}

			if !foundMusic {
				if currentPlaying != "" {
					// Limpa o status no Discord se a música parar ou fechar a aba
					client.SetActivity(client.Activity{})
					currentPlaying = ""
				}
				time.Sleep(5 * time.Second)
				continue
			}

			if songState != currentPlaying {
				currentPlaying = songState

				if foundMusic && activeWsURL != "" {
					lyrics.StartOrUpdate(songState, activeWsURL)
				}

				songStartTime = time.Now().Add(-time.Duration(finalCurrentTime) * time.Second)
			}

			err = client.SetActivity(client.Activity{
				Details:	statusDetails,
				State:		songState,
				LargeImage:	currentImage,
				Timestamps: &client.Timestamps{
					Start: &songStartTime,
				},
			})

			if err != nil {
				log.Printf("Erro ao atualizar Discord: %v", err)
			}

			time.Sleep(5 * time.Second)
		}
	}()

	lyrics.ShowUI()
}
