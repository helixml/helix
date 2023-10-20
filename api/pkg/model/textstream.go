package model

import (
	"bufio"
	"context"
	"io"
	"log"
	"strings"
)

// a configurable text stream to process llm output
type TextStream struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	// this is normally splitOnSpace
	splitter bufio.SplitFunc
	start    string
	ignore   string
	Output   chan string
}

func NewTextStream(
	splitter bufio.SplitFunc,
	start string,
	ignore string,
) *TextStream {
	reader, writer := io.Pipe()
	stream := &TextStream{
		reader:   reader,
		writer:   writer,
		splitter: splitter,
		start:    start,
		ignore:   ignore,
		Output:   make(chan string),
	}
	return stream
}

func (stream *TextStream) Write(data []byte) {
	stream.writer.Write(data)
}

// designed to be run in a goroutine
func (stream *TextStream) Start(ctx context.Context) {
	foundStartString := false
	scanner := bufio.NewScanner(stream.reader)
	scanner.Split(splitOnSpace)
	for scanner.Scan() {
		word := scanner.Text()
		if stream.start == "" || foundStartString {
			word = strings.TrimSuffix(word, stream.ignore)
			stream.Output <- word + " "
		} else {
			log.Printf("output: %s", word)
		}
		if strings.HasSuffix(word, stream.start) {
			foundStartString = true
		}
	}
}
