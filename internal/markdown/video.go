package markdown

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	youTubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	vimeoID   = regexp.MustCompile(`^[0-9]+$`)
	vimeoHash = regexp.MustCompile(`^[0-9a-f]+$`)
)

// videoPlayer maps a YouTube or Vimeo page URL, as written in
// ![title](https://youtube.com/watch?v=…), to the address of its embedded
// player. YouTube plays from youtube-nocookie.com and Vimeo with dnt=1, so
// neither sets tracking cookies until the reader presses play.
func videoPlayer(dest string) (string, bool) {
	u, err := url.Parse(dest)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	q := u.Query()
	switch host {
	case "youtube.com", "youtube-nocookie.com", "youtu.be":
		var id string
		switch {
		case host == "youtu.be":
			id = parts[0]
		case len(parts) == 1 && parts[0] == "watch":
			id = q.Get("v")
		case len(parts) == 2 && (parts[0] == "embed" || parts[0] == "shorts" || parts[0] == "live" || parts[0] == "v"):
			id = parts[1]
		}
		if !youTubeID.MatchString(id) {
			return "", false
		}
		player := "https://www.youtube-nocookie.com/embed/" + id
		start := youTubeStart(q.Get("t"))
		if start == 0 {
			start = youTubeStart(q.Get("start"))
		}
		if start > 0 {
			player += "?start=" + strconv.Itoa(start)
		}
		return player, true
	case "vimeo.com", "player.vimeo.com":
		if len(parts) > 0 && parts[0] == "video" {
			parts = parts[1:]
		}
		if len(parts) == 0 || !vimeoID.MatchString(parts[0]) {
			return "", false
		}
		v := url.Values{"dnt": {"1"}}
		if len(parts) > 1 && vimeoHash.MatchString(parts[1]) {
			v.Set("h", parts[1]) // unlisted video
		} else if h := q.Get("h"); vimeoHash.MatchString(h) {
			v.Set("h", h)
		}
		return "https://player.vimeo.com/video/" + parts[0] + "?" + v.Encode(), true
	}
	return "", false
}

// youTubeStart reads a start time: "90", "90s", "1m30s" or "1h2m3s".
func youTubeStart(t string) int {
	if t == "" {
		return 0
	}
	if n, err := strconv.Atoi(t); err == nil && n > 0 {
		return n
	}
	total, num := 0, 0
	for _, r := range t {
		switch {
		case r >= '0' && r <= '9':
			num = num*10 + int(r-'0')
		case r == 'h':
			total, num = total+num*3600, 0
		case r == 'm':
			total, num = total+num*60, 0
		case r == 's':
			total, num = total+num, 0
		default:
			return 0
		}
	}
	return total + num
}
