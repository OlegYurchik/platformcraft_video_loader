package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
	"net/url"

	"golang.org/x/net/html"
)

type empty struct{}

func loadChunk(chunkUrl *url.URL, attempts int, doneCh chan empty, dataCh chan<-[]byte) {
	defer func(){<-doneCh}()

	action := func(chunkUrl *url.URL, dataCh chan<-[]byte) error {
		response, err := http.Get(chunkUrl.String())
		if err != nil {
			return err
		}
		if response.StatusCode != 200 {
			return errors.New("Invalid status code: '" + response.Status + "'")
		}
		if err != nil {
			return err
		}

		data, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return err
		}
		dataCh <- data

		return nil
	}

	var attempt int = 0
	var multiplier float32 = 1
	err := action(chunkUrl, dataCh)
	for err != nil && attempt < attempts {
		fmt.Fprintln(
			os.Stderr,
			fmt.Sprintf("Get error from '%s': %s", chunkUrl.String(), err.Error()),
		)
		time.Sleep(1000 * time.Duration(multiplier) * time.Millisecond)
		attempt++
		multiplier *= rand.Float32() * 10.0
		err = action(chunkUrl, dataCh)
	}
	if err != nil {
		panic(err)
	}
}

func loadVideo(reader io.Reader, chunkListUrl *url.URL, attempts int, routines int) error {
	scanner := bufio.NewScanner(reader)
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

	var wg sync.WaitGroup
	doneCh := make(chan empty, routines)
	dataChs := []chan[]byte{}
	for index := 0; index < len(chunkUrlsList); index++ {
		dataCh := make(chan []byte, 1)
		dataChs = append(dataChs, dataCh)
	}
	go func() {
		wg.Add(1)
		for _, dataCh := range dataChs {
			os.Stdout.Write(<-dataCh)
		}
		wg.Done()
	}()

	for index, chunkUrl := range chunkUrlsList {
		doneCh <- empty{}
		go loadChunk(chunkUrl, attempts, doneCh, dataChs[index])
	}
	wg.Wait()

	return nil
}

func getChunkListUrl(reader io.Reader, resolution string) (*url.URL, error) {
	var chunkListUrl *url.URL
	var err error

	prefix := "#EXT-X-STREAM-INF:"
	scanner := bufio.NewScanner(reader)
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
				return nil, err
			}
			return chunkListUrl, nil
		}
	}
	if err := scanner.Err(); err != nil {
        return nil, err
    }
	return nil, errors.New(
		fmt.Sprintf("Have no chunklist url with resolution '%s'", resolution),
	)
}

func getPlaylistUrl(reader io.Reader, pageUrl *url.URL) (*url.URL, error) {
	var keyBytes, valueBytes []byte
	var key, value string
	tokenizer := html.NewTokenizer(reader)

	for {
		tokenType := tokenizer.Next()
		switch tokenType {
			case html.ErrorToken:
				return nil, tokenizer.Err()
			case html.StartTagToken:
				tagName, hasAttr := tokenizer.TagName()
				if string(tagName) == "source" && hasAttr {
					for key != "src" && hasAttr {
						keyBytes, valueBytes, hasAttr = tokenizer.TagAttr()
						key, value = string(keyBytes), string(valueBytes)
					}
					if key == "src" {
						playlistUrl, err := url.Parse(value)
						if err != nil {
							return nil, err
						}
						playlistUrl = pageUrl.ResolveReference(playlistUrl)
						return playlistUrl, nil
					}
				}
		}
	}
	return nil, errors.New("Playlist URL is not found")
}

func main() {
	var attempts int = 3
	var routines int = 1
	var err error

	// Check arguments coount
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "URL argument required!")
	}
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "RES argument required!")
		fmt.Fprintln(
			os.Stderr,
			"Format: platformcraft_video_loader <URL> <RES> [<ROUTINES>] [<ATTEMPTS>]",
		)
		fmt.Fprintln(os.Stderr, "Default values: ROUTINES=1, ATTEMPTS=3")
		os.Exit(1)
	}

	// Getting arguments values
	pageUrl, err := url.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "URL incorrect")
		os.Exit(1)
	}
	resolution := os.Args[2]
	if len(os.Args) < 4 {
		routines, err = strconv.Atoi(os.Args[3])
		if err != nil {
			panic(err)
		}
		if routines < 1 {
			fmt.Fprintln(os.Stderr, "ROUTINES must be more than 0")
			os.Exit(1)
		}
	}
	if len(os.Args) == 5 {
		attempts, err = strconv.Atoi(os.Args[4])
		if err != nil {
			panic(err)
		}
		if attempts < 1 {
			fmt.Fprintln(os.Stderr, "ATTEMPTS must be more than 0")
			os.Exit(1)
		}
	}

	response, err := http.Get(pageUrl.String())
	if err != nil {
		panic(err)
	}
	playlistUrl, err := getPlaylistUrl(response.Body, pageUrl)
	if err != nil {
		panic(err)
	}
	response, err = http.Get(playlistUrl.String())
	if err != nil {
		panic(err)
	}
	chunkListUrl, err := getChunkListUrl(response.Body, resolution)
	if err != nil {
		panic(err)
	}
	response, err = http.Get(chunkListUrl.String())
	if err != nil {
		panic(err)
	}
	err = loadVideo(response.Body, chunkListUrl, attempts, routines)
	if err != nil {
		panic(err)
	}
}
