package connection

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/cloudfoundry-incubator/garden/transport"
	"github.com/pivotal-golang/lager"
)

type hijackFunc func(streamType string) (net.Conn, io.Reader, error)

type streamHandler struct {
	processPipeline *processStream
	log             lager.Logger
	wg              *sync.WaitGroup
}

func newStreamHandler(processPipeline *processStream, log lager.Logger) *streamHandler {
	return &streamHandler{
		processPipeline: processPipeline,
		log:             log,
		wg:              new(sync.WaitGroup),
	}
}

func (sh *streamHandler) streamIn(stdin io.Reader) {
	if stdin == nil {
		return
	}

	go func(processInputStream *processStream, stdin io.Reader, log lager.Logger) {
		processInputStreamWriter := &stdinWriter{processInputStream}
		if _, err := io.Copy(processInputStreamWriter, stdin); err == nil {
			processInputStreamWriter.Close()
		} else {
			log.Error("streaming-stdin-payload", err)
		}
	}(sh.processPipeline, stdin, sh.log)
}

func (sh *streamHandler) streamOut(streamWriter io.Writer, streamReader io.Reader) {
	sh.wg.Add(1)
	go func() {
		io.Copy(streamWriter, streamReader)
		sh.wg.Done()
	}()
}

func (sh *streamHandler) wait(decoder *json.Decoder) (int, error) {
	for {
		payload := &transport.ProcessPayload{}
		err := decoder.Decode(payload)
		if err != nil {
			sh.wg.Wait()
			return 0, fmt.Errorf("connection: decode failed: %s", err)
		}

		if payload.Error != nil {
			sh.wg.Wait()
			return 0, fmt.Errorf("connection: process error: %s", *payload.Error)
		}

		if payload.ExitStatus != nil {
			sh.wg.Wait()
			status := int(*payload.ExitStatus)
			return status, nil
		}

		// discard other payloads
	}
}