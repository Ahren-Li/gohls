/*

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with this program.  If not, see <http://www.gnu.org/licenses/>.

*/

package main

import "flag"
import "fmt"
import "io"
import "net/http"
import "net/url"
import "log"
import "os"
import "time"
import "github.com/golang/groupcache/lru"
import "strings"
import "github.com/kz26/m3u8"

const VERSION = "1.0.5"

const USER_AGENT = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/68.0.3440.84 Safari/537.36"

var client = &http.Client{}

func doRequest(c *http.Client, req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", USER_AGENT)
	resp, err := c.Do(req)
	return resp, err
}

type Download struct {
	URI string
	totalDuration time.Duration
}

func downloadSegment(fn string, dlc chan *Download, recTime time.Duration) {
	out, err := os.Create(fn)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	for v := range dlc {
		req, err := http.NewRequest("GET", v.URI, nil)
		if err != nil {
			log.Fatal(err)
		}
		resp, err := doRequest(client, req)
		if err != nil {
			log.Print(err)
			continue
		}
		if resp.StatusCode != 200 {
			log.Printf("Received HTTP %v for %v\n", resp.StatusCode, v.URI)
			continue
		}
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		resp.Body.Close()
		log.Printf("Downloaded %v\n", v.URI)
		if recTime != 0 {
			log.Printf("Recorded %v of %v\n", v.totalDuration, recTime)
			} else {
				log.Printf("Recorded %v\n", v.totalDuration)
			}
	}
}

func getPlaylist(urlStr string, recTime time.Duration, useLocalTime bool, dlc chan *Download) {
	startTime := time.Now()
	var recDuration time.Duration = 0
	cache := lru.New(1024)
	playlistUrl, err := url.Parse("https://media.wanmen.org/")
	if err != nil {
		log.Fatal(err)
	}
	for {
        file,err := os.Open(urlStr)
        if err != nil {
            log.Fatal(err)
        }
        defer file.Close()

		playlist, listType, err := m3u8.DecodeFrom(file, true)
		if err != nil {
			log.Fatal(err)
		}
		if listType == m3u8.MEDIA {
			mpl := playlist.(*m3u8.MediaPlaylist)
			for _, v := range mpl.Segments {
				if v != nil {
					var msURI string
					if strings.HasPrefix(v.URI, "http") {
						msURI, err = url.QueryUnescape(v.URI)
						if err != nil {
							log.Fatal(err)
						}
					} else {
						msUrl, err := playlistUrl.Parse(v.URI)
						if err != nil {
							log.Print(err)
							continue
						}
						msURI, err = url.QueryUnescape(msUrl.String())
						if err != nil {
							log.Fatal(err)
						}
					}
					_, hit := cache.Get(msURI)
					if !hit {
						cache.Add(msURI, nil)
						if useLocalTime {
							recDuration = time.Now().Sub(startTime)
						} else {
							recDuration += time.Duration(int64(v.Duration * 1000000000))
						}
						dlc <- &Download{msURI, recDuration}
					}
					if recTime != 0 && recDuration != 0 && recDuration >= recTime {
						close(dlc)
						return
					}
				}
			}
			if mpl.Closed {
					close(dlc)
					return
			} else {
				time.Sleep(time.Duration(int64(mpl.TargetDuration * 1000000000)))
			}
		} else {
			log.Fatal("Not a valid media playlist")
		}
	}
}

func main() {

	duration := flag.Duration("t", time.Duration(0), "Recording duration (0 == infinite)")
	useLocalTime := flag.Bool("l", false, "Use local time to track duration instead of supplied metadata")
	flag.Parse()

	os.Stderr.Write([]byte(fmt.Sprintf("gohls %v - HTTP Live Streaming (HLS) downloader\n", VERSION)))
	os.Stderr.Write([]byte("Copyright (C) 2013-2014 Kevin Zhang. Licensed for use under the GNU GPL version 3.\n"))

	msChan := make(chan *Download, 1024)
	go getPlaylist(flag.Arg(0), *duration, *useLocalTime, msChan)

	out := strings.Replace(flag.Arg(0), "m3u8", "ts", -1)
	downloadSegment(out, msChan, *duration)
}
