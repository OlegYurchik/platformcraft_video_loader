package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"math/rand"
	"strconv"
	"strings"
	"time"
	"net/url"

	"golang.org/x/net/html"
)

func loadChunk(chunkUrl *url.URL, attempts int) {
	action := func(chunkUrl *url.URL) error {
		bodyReader, err := func() (io.Reader, error) {
			response, err := http.Get(chunkUrl.String())
			if err != nil {
				return nil, err
			}
			if response.StatusCode != 200 {
				return nil, errors.New("Invalid status code: '" + response.Status + "'")
			}
			return response.Body, nil
		}()
		if err != nil {
			return err
		}

		_, err = io.Copy(os.Stdout, bodyReader)
		if err != nil {
			return err
		}
		return nil
	}

	var attempt int = 0
	var multiplier float32 = 1
	err := action(chunkUrl)
	for err != nil && attempt < attempts {
		fmt.Fprintln(
			os.Stderr,
			fmt.Sprintf("Get error from '%s': %s", chunkUrl.String(), err.Error()),
		)
		time.Sleep(1000 * time.Duration(multiplier) * time.Millisecond)
		attempt++
		multiplier *= rand.Float32() * 10.0
		err = action(chunkUrl)
	}
	if err != nil {
		panic(err)
	}
}

func loadVideo(playlistUrl *url.URL, resolution string, attempts int) error {
	response, err := http.Get(playlistUrl.String())
	if err != nil {
		return err
	}

	prefix := "#EXT-X-STREAM-INF:"
	scanner := bufio.NewScanner(response.Body)
	var chunkListUrl *url.URL
	foundChunkListUrl := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			for _, pairString := range strings.Split(line[len(prefix):], ",") {
				pair := strings.Split(pairString, "=")
				if len(pair) < 2 {
					continue
				}

				key, value := pair[0], pair[1]
				if key == "RESOLUTION" && value == resolution {
					foundChunkListUrl = true
					break
				}
			}
		} else if line[0] != '#' && foundChunkListUrl {
			chunkListUrl, err = url.Parse(line)
			if err != nil {
				return err
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
        return err
    }
	if chunkListUrl == nil {
		return errors.New(
			fmt.Sprintf("Have no chunklist url with resolution '%s'", resolution),
		)
	}

	response, err = http.Get(chunkListUrl.String())
	if err != nil {
		return err
	}

	scanner = bufio.NewScanner(response.Body)
	chunkUrlsList := []*url.URL{}
	for scanner.Scan() {
		line := scanner.Text()
		if line[0] != '#' {
			lineUrl, err := url.Parse(line)
			if err != nil {
				return err
			}
			lineUrl = chunkListUrl.ResolveReference(lineUrl)
			chunkUrlsList = append(chunkUrlsList, lineUrl)
		}
	}

	for _, chunkUrl := range chunkUrlsList {
		loadChunk(chunkUrl, attempts)
	}

	return nil
}

func getPlaylistUrl(reader io.Reader, scheme string) (*url.URL, error) {
	tokenizer := html.NewTokenizer(reader)

	for {
		tokenType := tokenizer.Next()
		switch tokenType {
			case html.ErrorToken:
				return nil, tokenizer.Err()
			case html.StartTagToken:
				tagNameBytes, hasAttr := tokenizer.TagName()
				tagName := string(tagNameBytes)
				if tagName == "source" && hasAttr {
					var keyBytes, valueBytes []byte
					key, value, moreAttr := "", "", true
					for key != "src" && moreAttr {
						keyBytes, valueBytes, moreAttr = tokenizer.TagAttr()
						key, value = string(keyBytes), string(valueBytes)
					}
					if key == "src" {
						playlistUrl, err := url.Parse(scheme + ":" + value)
						if err != nil {
							return nil, err
						}
						return playlistUrl, nil
					}
				}
		}
	}
	return nil, errors.New("Playlist URL is not found")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "URL argument required!")
	}
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "RES argument required!")
		fmt.Fprintln(
			os.Stderr,
			"Format: platformcraft_video_loader <URL> <RES> [<ATTEMPTS>] [<ROUTINES>]",
		)
		fmt.Fprintln(os.Stderr, "Default values: ATTEMPTS=10, ROUTINES=1")
		os.Exit(1)
	}

	var attempts int = 10
	var routines int = 1
	var err error
	pageUrl, err := url.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "URL incorrect")
		os.Exit(1)
	}
	resolution := os.Args[2]
	if len(os.Args) > 3 {
		attempts, err = strconv.Atoi(os.Args[3])
		if err != nil {
			panic(err)
		}
	}
	if attempts < 1 {
		fmt.Fprintln(os.Stderr, "ATTEMPTS must be more than 0")
		os.Exit(1)
	}
	if len(os.Args) == 5 {
		routines, err = strconv.Atoi(os.Args[4])
		if err != nil {
			panic(err)
		}
	}
	if routines < 1 {
		fmt.Fprintln(os.Stderr, "ROUTINES must be more than 0")
		os.Exit(1)
	}

	response, err := http.Get(pageUrl.String())
	if err != nil {
		panic(err)
	}
	playlistUrl, err := getPlaylistUrl(response.Body, pageUrl.Scheme)
	if err != nil {
		panic(err)
	}
	err = loadVideo(playlistUrl, resolution, attempts)
	if err != nil {
		panic(err)
	}
}
