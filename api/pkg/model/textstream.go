package model

import (
	"bufio"
	"fmt"
	"io"
)

// a configurable text stream to process llm output
type TextStream struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	// this is normally splitOnSpace
	splitter bufio.SplitFunc
	handler  func(chunk string)
}

func NewTextStream(
	splitter bufio.SplitFunc,
	handler func(chunk string),
) *TextStream {
	reader, writer := io.Pipe()
	stream := &TextStream{
		reader:   reader,
		writer:   writer,
		splitter: splitter,
		handler:  handler,
	}
	return stream
}

func (stream *TextStream) Write(data []byte) (int, error) {
	n, err := stream.writer.Write(data)
	if err != nil {
		return n, fmt.Errorf("error writing to stream: %s", err)
	}
	return n, nil
}

// designed to be run in a goroutine
func (stream *TextStream) Start() {
	scanner := bufio.NewScanner(stream.reader)
	scanner.Split(stream.splitter)
	for scanner.Scan() {
		stream.handler(scanner.Text())
	}
}

func (stream *TextStream) Close() error {
	err := stream.reader.Close()
	if err != nil {
		return err
	}
	err = stream.writer.Close()
	if err != nil {
		return err
	}
	return nil
}
