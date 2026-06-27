package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"brave-scraper/lyrics"

	"github.com/gorilla/websocket"
	"github.com/hugolgst/rich-go/client"
)

type Target struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	Type                 string `json:"type"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
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
		return "https://raw.githubusercontent.com/zkingboos/browser-music-rpc/main/assets/default.png"
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ArtworkUrl100 string `json:"artworkUrl100"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "https://raw.githubusercontent.com/zkingboos/browser-music-rpc/main/assets/default.png"
	}

	if len(result.Results) > 0 {
		bigImg := strings.Replace(result.Results[0].ArtworkUrl100, "100x100bb", "512x512bb", 1)
		artworkCache[songName] = bigImg
		return bigImg
	}

	defaultIcon := "https://raw.githubusercontent.com/zkingboos/browser-music-rpc/main/assets/default.png"
	artworkCache[songName] = defaultIcon
	return defaultIcon
}

func getMediaInfo(wsURL string) (isPlaying bool, currentTime float64, docTitle string, channel string, thumb string) {
	if wsURL == "" {
		return false, 0, "", "", ""
	}
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return false, 0, "", "", ""
	}
	defer c.Close()

	req := map[string]interface{}{
		"id":     1,
		"method": "Runtime.evaluate",
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
				let thumb = '';
				let url = window.location.href;
				if (url.includes('youtube.com/watch')) {
					let params = new URLSearchParams(window.location.search);
					if (params.has('v')) {
						thumb = 'https://i.ytimg.com/vi/' + params.get('v') + '/hqdefault.jpg';
					}
				}
				if (!thumb) {
					let imgEl = document.querySelector('img.image.ytmusic-player-bar') || document.querySelector('img[data-testid="cover-art-image"]');
					if (imgEl && imgEl.src) thumb = imgEl.src;
				}
				if (!thumb) {
					let scEl = document.querySelector('.playbackSoundBadge__avatar span.sc-artwork');
					if (scEl) {
						let bg = window.getComputedStyle(scEl).backgroundImage;
						if (bg && bg !== 'none') {
							thumb = bg.replace(/url\(['"]?(.*?)['"]?\)/i, '$1');
						}
					}
				}
				return JSON.stringify({isPlaying, currentTime, docTitle, channel, thumb});
			})()`,
		},
	}

	c.SetWriteDeadline(time.Now().Add(1 * time.Second))
	if err := c.WriteJSON(req); err != nil {
		return false, 0, "", "", ""
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
		return false, 0, "", "", ""
	}

	var result struct {
		IsPlaying   bool    `json:"isPlaying"`
		CurrentTime float64 `json:"currentTime"`
		DocTitle    string  `json:"docTitle"`
		Channel     string  `json:"channel"`
		Thumb       string  `json:"thumb"`
	}
	json.Unmarshal([]byte(resp.Result.Result.Value), &result)

	return result.IsPlaying, result.CurrentTime, result.DocTitle, result.Channel, result.Thumb
}

func startMonitoring() {
	var currentPlaying string
	var songStartTime time.Time

	for {
		resp, err := http.Get("http://127.0.0.1:9222/json/list")
		if err != nil {
			log.Printf("Waiting for browser... (Error: %v)\n", err)
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

				playing, cTime, docTitle, channel, thumb := getMediaInfo(wsURL)

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

				statusDetails = "🎵 Listening on YouTube"
				songState = cleanTitle
				if thumb != "" && !strings.HasPrefix(thumb, "data:image") {
					currentImage = thumb
				} else {
					currentImage = getArtwork(songState)
				}

				activeWsURL = wsURL
				finalCurrentTime = cTime
				foundMusic = true
				break
			} else if strings.Contains(pageURL, "open.spotify.com") {
				playing, cTime, docTitle, _, thumb := getMediaInfo(t.WebSocketDebuggerURL)
				if !playing {
					continue
				}
				statusDetails = "🎵 Listening on Spotify"
				songState = strings.ReplaceAll(docTitle, " - Spotify", "")
				if thumb != "" && !strings.HasPrefix(thumb, "data:image") {
					currentImage = thumb
				} else {
					currentImage = getArtwork(songState)
				}
				activeWsURL = t.WebSocketDebuggerURL
				finalCurrentTime = cTime
				foundMusic = true
				break
			} else if strings.Contains(pageURL, "soundcloud.com") {
				playing, cTime, docTitle, _, thumb := getMediaInfo(t.WebSocketDebuggerURL)
				if !playing {
					continue
				}
				statusDetails = "🎵 Listening on SoundCloud"
				songState = docTitle
				if thumb != "" && !strings.HasPrefix(thumb, "data:image") {
					currentImage = thumb
				} else {
					currentImage = getArtwork(songState)
				}
				activeWsURL = t.WebSocketDebuggerURL
				finalCurrentTime = cTime
				foundMusic = true
				break
			}
		}

		if !foundMusic {
			if currentPlaying != "" {
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
			Details:    statusDetails,
			State:      songState,
			LargeImage: currentImage,
			Timestamps: &client.Timestamps{
				Start: &songStartTime,
			},
		})

		if err != nil {
			log.Printf("Error updating Discord: %v", err)
		}

		time.Sleep(5 * time.Second)
	}
}
