package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"math/rand"
	"strings"
	"time"
	"net/url"

	"golang.org/x/net/html"
)

func loadChunk(chunkUrl *url.URL, attempts byte) {
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

	var attempt byte = 0
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

func loadVideo(playlistUrl *url.URL, resolution string) error {
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
		loadChunk(chunkUrl, 10)
	}

	return nil
}

func getPlaylistUrl(reader io.Reader, scheme string) (*url.URL, error) {
	tokenizer := html.NewTokenizer(reader)

	video_flag := false
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
			case html.ErrorToken:
				return nil, tokenizer.Err()
			case html.StartTagToken:
				tagNameBytes, hasAttr := tokenizer.TagName()
				tagName := string(tagNameBytes)
				if tagName == "video" && hasAttr {
					var keyBytes, valueBytes []byte
					key, value, moreAttr := "", "", true
					for key != "id" && moreAttr {
						keyBytes, valueBytes, moreAttr = tokenizer.TagAttr()
						key, value = string(keyBytes), string(valueBytes)
					}
					video_flag = (key == "id" && value == "video")
				} else if video_flag && tagName == "source" && hasAttr {
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
			case html.EndTagToken:
				tagNameBytes, _ := tokenizer.TagName()
				tagName := string(tagNameBytes)
				if video_flag && tagName == "video" {
					video_flag = false
				}
		}
	}
	return nil, errors.New("Playlist URL is not found")
}

func main() {
	errorMessages := []string{}
	if len(os.Args) < 2 {
		errorMessages = append(errorMessages, "URL argument required!")
	}
	if len(os.Args) < 3 {
		errorMessages = append(errorMessages, "RES argument required!")
	}
	if len(errorMessages) > 0 {
		for _, errorMessage := range errorMessages {
			fmt.Fprintln(os.Stderr, errorMessage)
		}
		fmt.Fprintln(os.Stderr, "Format: platformcraft_video_loader <URL> <RES>")
		os.Exit(1)
	}

	pageUrl, err := url.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "URL incorrect")
		os.Exit(1)
	}
	resolution := os.Args[2]

	response, err := http.Get(pageUrl.String())
	if err != nil {
		panic(err)
	}
	playlistUrl, err := getPlaylistUrl(response.Body, pageUrl.Scheme)
	if err != nil {
		panic(err)
	}
	err = loadVideo(playlistUrl, resolution) // 1280x720
	if err != nil {
		panic(err)
	}
}
