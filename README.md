# Discord Web Presence (Rich Presence + Synced Lyrics)

> Showcase your browser music as your Discord status with real-time synchronized floating lyrics.

This project is a cross-platform Go tool that connects to your browser (via the debugging port) to extract information about the music you are listening to and display it on your Discord profile (Rich Presence). Additionally, it features a floating widget that shows real-time synchronized lyrics on your screen.

## How it works

The script monitors your browser tabs looking for music services. Currently, it supports:
- YouTube
- YouTube Music
- Spotify Web
- SoundCloud

When a song is identified, the program does two things:
1. Automatically updates your Discord status, displaying the song name, album art (fetched via the iTunes API), and playback time.
2. Fetches synchronized lyrics from LRCLib and displays a small floating window on your desktop, in a "Dynamic Island" style, that follows along with the song.

## Prerequisites

1. Have Go installed on your computer.
2. The browser (Brave, Chrome, etc.) must be started with the debugging port open on `9222`.
   - Windows shortcut example: `chrome.exe --remote-debugging-port=9222`
   - macOS terminal example: `/Applications/Brave\ Browser.app/Contents/MacOS/Brave\ Browser --remote-debugging-port=9222`
3. A "Client ID" from an application created in the [Discord Developer Portal](https://discord.com/developers/applications).

## How to run

1. Clone this repository.
2. Set your environment variable with your Discord Application ID:
   ```bash
   export DISCORD_CLIENT_ID="your_id_here"
   ```
3. Download the project dependencies:
   ```bash
   go mod tidy
   ```
4. Run the main program:
   ```bash
   go run main.go
   ```

## Main Dependencies

- [gorilla/websocket](https://github.com/gorilla/websocket): For communicating with the browser.
- [rich-go](https://github.com/hugolgst/rich-go): For Discord Rich Presence integration.
- [fyne](https://fyne.io/): For building the graphical user interface (lyrics window).
