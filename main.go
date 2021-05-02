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
	"strings"
	"sync"
	"time"
	"net/url"

	"golang.org/x/net/html"
	"github.com/akamensky/argparse"
)

type empty struct{}

func loadChunk(chunkUrl *url.URL, attempts int, doneCh <-chan empty, dataCh chan<-[]byte) {
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
		response.Body.Close()
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
		attempt++
		fmt.Fprintln(
			os.Stderr,
			fmt.Sprintf("Get error from '%s'", chunkUrl.String()),
		)
		fmt.Fprintln(os.Stderr, err.Error())
		fmt.Fprintln(os.Stderr, fmt.Sprintf("Retry: attempt %d", attempt))
		time.Sleep(1000 * time.Duration(multiplier) * time.Millisecond)
		multiplier *= rand.Float32() * 10.0
		err = action(chunkUrl, dataCh)
	}
	if err != nil {
		panic(err)
	}
}

func loadVideo(reader io.ReadCloser, chunkListUrl *url.URL, attempts int, goroutines int) error {
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
	if err := scanner.Err(); err != nil {
        return err
    }
	reader.Close()

	var wg sync.WaitGroup
	doneCh := make(chan empty, goroutines)
	dataChs := []chan[]byte{}
	for index := 0; index < len(chunkUrlsList); index++ {
		dataCh := make(chan []byte, 1)
		dataChs = append(dataChs, dataCh)
	}
	go func() {
		wg.Add(1)
		for _, dataCh := range dataChs {
			os.Stdout.Write(<-dataCh)
			close(dataCh)
		}
		wg.Done()
	}()

	for index, chunkUrl := range chunkUrlsList {
		doneCh <- empty{}
		go loadChunk(chunkUrl, attempts, doneCh, dataChs[index])
	}
	wg.Wait()
	close(doneCh)

	return nil
}

func getChunkListUrl(reader io.ReadCloser, resolution string) (*url.URL, error) {
	defer reader.Close()

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

func getPlaylistUrl(reader io.ReadCloser, pageUrl *url.URL) (*url.URL, error) {
	defer reader.Close()

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
				// Get attributes from "source" tag
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
	parser := argparse.NewParser("platformcraft_video_loader", "Video loader from PlatformCraft")
	pageUrlString := parser.String(
		"u", "url",
		&argparse.Options{
			Required: true,
			Help: "Video URL for downloading",
		},
	)
	resolution := parser.String(
		"r", "resolution",
		&argparse.Options{
			Required: true,
			Help: "Set video resolution",
		},
	)
	goroutines := parser.Int(
		"g", "goroutines",
		&argparse.Options{
			Required: false,
			Help: "Max count of parallel working goroutines",
			Default: 1,
		},
	)
	attempts := parser.Int(
		"a", "attempts",
		&argparse.Options{
			Required: false,
			Help: "Count of retry attempts if downloaded is failed",
			Default: 3,
		},
	)

	err := parser.Parse(os.Args)
	if err != nil {
		panic(err)
	}

	pageUrl, err := url.Parse(*pageUrlString)
	if err != nil {
		fmt.Fprintln(os.Stderr, "URL incorrect")
		os.Exit(1)
	}
	if *goroutines < 1 {
		fmt.Fprintln(os.Stderr, "GOROUTINES must be more than 0")
		os.Exit(1)
	}
	if *attempts < 1 {
		fmt.Fprintln(os.Stderr, "ATTEMPTS must be more than 0")
		os.Exit(1)
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
	chunkListUrl, err := getChunkListUrl(response.Body, *resolution)
	if err != nil {
		panic(err)
	}
	response, err = http.Get(chunkListUrl.String())
	if err != nil {
		panic(err)
	}
	err = loadVideo(response.Body, chunkListUrl, *attempts, *goroutines)
	if err != nil {
		panic(err)
	}
}
