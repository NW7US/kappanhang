package main

import (
	"bytes"
	"errors"
	"os"
	"sync"

	"github.com/akosmarton/papipes"
	"github.com/nonoo/kappanhang/log"
)

type audioStruct struct {
	source papipes.Source
	sink   papipes.Sink

	// Send to this channel to play audio.
	play chan []byte
	rec  chan []byte

	mutex   sync.Mutex
	playBuf *bytes.Buffer
	canPlay chan bool
}

var audio audioStruct

func (a *audioStruct) playLoop() {
	for {
		<-a.canPlay

		for {
			a.mutex.Lock()
			if a.playBuf.Len() < 1920 {
				a.mutex.Unlock()
				break
			}

			d := make([]byte, 1920)
			bytesToWrite, err := a.playBuf.Read(d)
			a.mutex.Unlock()
			if err != nil {
				log.Error(err)
				break
			}
			if bytesToWrite != len(d) {
				log.Error("buffer underread")
				break
			}

			for {
				written, err := a.source.Write(d)
				if err != nil {
					if _, ok := err.(*os.PathError); !ok {
						log.Error(err)
					}
					return
				}
				bytesToWrite -= written
				if bytesToWrite == 0 {
					break
				}
				d = d[written:]
			}
		}
	}
}

func (a *audioStruct) recLoop() {
	frameBuf := make([]byte, 1920)
	buf := bytes.NewBuffer([]byte{})

	for {
		n, err := a.sink.Read(frameBuf)
		if err != nil {
			if _, ok := err.(*os.PathError); !ok {
				log.Error(err)
			}
			return
		}

		buf.Write(frameBuf[:n])

		for buf.Len() >= len(frameBuf) {
			n, err = buf.Read(frameBuf)
			if err != nil {
				exit(err)
			}
			if n != len(frameBuf) {
				exit(errors.New("buffer read error"))
			}
			a.rec <- frameBuf
		}
	}
}

func (a *audioStruct) loop() {
	go a.playLoop()
	go a.recLoop()

	for {
		d := <-a.play
		a.mutex.Lock()
		a.playBuf.Write(d)
		a.mutex.Unlock()

		select {
		case a.canPlay <- true:
		default:
		}
	}
}

func (a *audioStruct) init() {
	a.source.Name = "kappanhang"
	a.source.Filename = "/tmp/kappanhang.source"
	a.source.Rate = 48000
	a.source.Format = "s16le"
	a.source.Channels = 1
	a.source.SetProperty("device.buffering.buffer_size", (48000*16)/10) // 100 ms
	a.source.SetProperty("device.description", "kappanhang input")

	a.sink.Name = "kappanhang"
	a.sink.Filename = "/tmp/kappanhang.sink"
	a.sink.Rate = 48000
	a.sink.Format = "s16le"
	a.sink.Channels = 1
	a.sink.SetProperty("device.buffering.buffer_size", (48000*16)/10)
	a.sink.SetProperty("device.description", "kappanhang output")

	if err := a.source.Open(); err != nil {
		exit(err)
	}

	if err := a.sink.Open(); err != nil {
		exit(err)
	}

	a.playBuf = bytes.NewBuffer([]byte{})
	a.play = make(chan []byte)
	a.canPlay = make(chan bool)
	a.rec = make(chan []byte)
	go a.loop()
}

func (a *audioStruct) deinit() {
	if a.source.IsOpen() {
		if err := a.source.Close(); err != nil {
			if _, ok := err.(*os.PathError); !ok {
				log.Error(err)
			}
		}
	}

	if a.sink.IsOpen() {
		if err := a.sink.Close(); err != nil {
			if _, ok := err.(*os.PathError); !ok {
				log.Error(err)
			}
		}
	}
}