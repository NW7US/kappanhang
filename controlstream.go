package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

type controlStream struct {
	common streamCommon
	serial serialStream
	audio  audioStream

	deinitNeededChan   chan bool
	deinitFinishedChan chan bool

	authInnerSendSeq uint16
	authID           [6]byte

	gotAuthID                  bool
	serialAndAudioStreamOpened bool
	deinitializing             bool

	secondAuthTimer              *time.Timer
	requestSerialAndAudioTimeout *time.Timer
}

func (s *controlStream) sendPktLogin() error {
	// The reply to the auth packet will contain a 6 bytes long auth ID with the first 2 bytes set to our ID.
	authStartID := []byte{0x63, 0x00}
	p := []byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		byte(s.common.localSID >> 24), byte(s.common.localSID >> 16), byte(s.common.localSID >> 8), byte(s.common.localSID),
		byte(s.common.remoteSID >> 24), byte(s.common.remoteSID >> 16), byte(s.common.remoteSID >> 8), byte(s.common.remoteSID),
		0x00, 0x00, 0x00, 0x70, 0x01, 0x00, 0x00, byte(s.authInnerSendSeq),
		byte(s.authInnerSendSeq >> 8), 0x00, authStartID[0], authStartID[1], 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2b, 0x3f, 0x55, 0x5c, 0x00, 0x00, 0x00, 0x00, // username: beer
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2b, 0x3f, 0x55, 0x5c, 0x3f, 0x25, 0x77, 0x58, // pass: beerbeer
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x69, 0x63, 0x6f, 0x6d, 0x2d, 0x70, 0x63, 0x00, // icom-pc in plain text
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if err := s.common.pkt0.sendTrackedPacket(&s.common, p); err != nil {
		return err
	}

	s.authInnerSendSeq++
	return nil
}

func (s *controlStream) sendPktAuth(magic byte) error {
	// Example request from PC:  0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0d, 0x00,
	//                           0xbb, 0x41, 0x3f, 0x2b, 0xe6, 0xb2, 0x7b, 0x7b,
	//                           0x00, 0x00, 0x00, 0x30, 0x01, 0x05, 0x00, 0x02,
	//                           0x00, 0x00, 0x5d, 0x37, 0x12, 0x82, 0x3b, 0xde,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
	// Example reply from radio: 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0e, 0x00,
	//                           0xe6, 0xb2, 0x7b, 0x7b, 0xbb, 0x41, 0x3f, 0x2b,
	//                           0x00, 0x00, 0x00, 0x30, 0x02, 0x05, 0x00, 0x02,
	//                           0x00, 0x00, 0x5d, 0x37, 0x12, 0x82, 0x3b, 0xde,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00

	p := []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		byte(s.common.localSID >> 24), byte(s.common.localSID >> 16), byte(s.common.localSID >> 8), byte(s.common.localSID),
		byte(s.common.remoteSID >> 24), byte(s.common.remoteSID >> 16), byte(s.common.remoteSID >> 8), byte(s.common.remoteSID),
		0x00, 0x00, 0x00, 0x30, 0x01, magic, 0x00, byte(s.authInnerSendSeq),
		byte(s.authInnerSendSeq >> 8), 0x00, s.authID[0], s.authID[1], s.authID[2], s.authID[3], s.authID[4], s.authID[5],
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if err := s.common.pkt0.sendTrackedPacket(&s.common, p); err != nil {
		return err
	}
	s.authInnerSendSeq++
	return nil
}

func (s *controlStream) sendRequestSerialAndAudio() error {
	log.Debug("requesting serial and audio stream")
	p := []byte{0x90, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		byte(s.common.localSID >> 24), byte(s.common.localSID >> 16), byte(s.common.localSID >> 8), byte(s.common.localSID),
		byte(s.common.remoteSID >> 24), byte(s.common.remoteSID >> 16), byte(s.common.remoteSID >> 8), byte(s.common.remoteSID),
		0x00, 0x00, 0x00, 0x80, 0x01, 0x03, 0x00, byte(s.authInnerSendSeq),
		byte(s.authInnerSendSeq >> 8), 0x00, s.authID[0], s.authID[1], s.authID[2], s.authID[3], s.authID[4], s.authID[5],
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10,
		0x80, 0x00, 0x00, 0x90, 0xc7, 0x0e, 0x86, 0x01, // The last 5 bytes from this row can be acquired from a reply starting with 0xa8 or 0x90
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x49, 0x43, 0x2d, 0x37, 0x30, 0x35, 0x00, 0x00, // IC-705 in plain text
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2b, 0x3f, 0x55, 0x5c, 0x00, 0x00, 0x00, 0x00, // username: beer
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x01, 0x01, 0x04, 0x04, 0x00, 0x00, 0xbb, 0x80,
		0x00, 0x00, 0xbb, 0x80, 0x00, 0x00, 0xc3, 0x52,
		0x00, 0x00, 0xc3, 0x53, 0x00, 0x00, 0x00, 0xa0,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if err := s.common.pkt0.sendTrackedPacket(&s.common, p); err != nil {
		return err
	}

	s.authInnerSendSeq++

	return nil
}

func (s *controlStream) parseNullTerminatedString(d []byte) (res string) {
	nullIndex := strings.Index(string(d), "\x00")
	if nullIndex > 0 {
		res = string(d[:nullIndex])
	}
	return
}

func (s *controlStream) handleRead(r []byte) error {
	switch len(r) {
	case 64:
		if bytes.Equal(r[:6], []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00}) {
			// Example answer from radio:   0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00,
			// 0x33, 0x60, 0xd4, 0xe5, 0xf4, 0x67, 0x86, 0xe1,
			// 0x00, 0x00, 0x00, 0x30, 0x02, 0x05, 0x00, 0x02,
			// 0x00, 0x00, 0x35, 0x34, 0x76, 0x11, 0xb9, 0xd0,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00

			log.Debug("auth ok")

			if r[21] == 0x05 && !s.serialAndAudioStreamOpened { // Answer for our second auth?
				s.secondAuthTimer.Stop()

				if err := s.sendRequestSerialAndAudio(); err != nil {
					reportError(err)
				}
			}
		}
	case 80:
		if bytes.Equal(r[:6], []byte{0x50, 0x00, 0x00, 0x00, 0x00, 0x00}) {
			// Example answer from radio: 0x50, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x00,
			//							  0x86, 0x1f, 0x2f, 0xcc, 0x03, 0x03, 0x89, 0x29,
			//							  0x00, 0x00, 0x00, 0x40, 0x02, 0x03, 0x00, 0x52,
			//							  0x00, 0x00, 0xf8, 0xad, 0x06, 0x8d, 0xda, 0x7b,
			//							  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10,
			//							  0x80, 0x00, 0x00, 0x90, 0xc7, 0x0e, 0x86, 0x01,
			//							  0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00,
			//							  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			//							  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			//							  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00

			if bytes.Equal(r[48:51], []byte{0xff, 0xff, 0xff}) {
				return errors.New("auth failed")
			}
			if bytes.Equal(r[48:51], []byte{0x00, 0x00, 0x00}) && r[64] == 0x01 {
				return errors.New("got radio disconnected")
			}
		}
	case 144:
		if !s.serialAndAudioStreamOpened && bytes.Equal(r[:6], []byte{0x90, 0x00, 0x00, 0x00, 0x00, 0x00}) && r[96] == 1 {
			// Example answer:
			// 0x90, 0x00, 0x00, 0x00, 0x00, 0x00, 0x19, 0x00,
			// 0xc6, 0x5f, 0x6f, 0x0c, 0x5f, 0x8b, 0x1e, 0x89,
			// 0x00, 0x00, 0x00, 0x80, 0x03, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x31, 0x30, 0x31, 0x47, 0x39, 0x07,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10,
			// 0x80, 0x00, 0x00, 0x90, 0xc7, 0x0e, 0x86, 0x01,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x49, 0x43, 0x2d, 0x37, 0x30, 0x35, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x01, 0x00, 0x00, 0x00, 0x69, 0x63, 0x6f, 0x6d,
			// 0x2d, 0x70, 0x63, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			// 0x00, 0x00, 0x00, 0x00, 0xc0, 0xa8, 0x03, 0x03,
			// 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00

			s.secondAuthTimer.Stop()
			s.requestSerialAndAudioTimeout.Stop()

			devName := s.parseNullTerminatedString(r[64:])
			log.Print("got serial and audio request success, device name: ", devName)

			// Stuff can change in the meantime because of a previous login...
			s.common.remoteSID = binary.BigEndian.Uint32(r[8:12])
			s.common.localSID = binary.BigEndian.Uint32(r[12:16])
			copy(s.authID[:], r[26:32])
			s.gotAuthID = true

			if err := s.serial.init(devName); err != nil {
				return errors.New("serial/" + err.Error())
			}

			if err := s.audio.init(devName); err != nil {
				return errors.New("audio/" + err.Error())
			}

			s.serialAndAudioStreamOpened = true
		}
	}
	return nil
}

func (s *controlStream) loop() {
	startTime := time.Now()

	s.secondAuthTimer = time.NewTimer(time.Second)
	reauthTicker := time.NewTicker(60 * time.Second)
	statusLogTicker := time.NewTicker(3 * time.Second)

	for {
		select {
		case <-s.secondAuthTimer.C:
			if err := s.sendPktAuth(0x05); err != nil {
				reportError(err)
			}
			log.Debug("second auth sent...")
		case r := <-s.common.readChan:
			if !s.deinitializing {
				if err := s.handleRead(r); err != nil {
					reportError(err)
				}
			}
		case <-reauthTicker.C:
			log.Debug("sending auth")
			if err := s.sendPktAuth(0x05); err != nil {
				reportError(err)
			}
		case <-statusLogTicker.C:
			if s.serialAndAudioStreamOpened {
				log.Print("running for ", time.Since(startTime), " roundtrip latency ", s.common.pkt7.latency)
			}
		case <-s.deinitNeededChan:
			s.deinitFinishedChan <- true
			return
		}
	}
}

func (s *controlStream) init() error {
	log.Debug("init")

	if err := s.common.init("control", 50001); err != nil {
		return err
	}

	if err := s.common.start(); err != nil {
		return err
	}

	s.common.pkt7.startPeriodicSend(&s.common, 2, false)
	s.common.pkt0.startPeriodicSend(&s.common)

	if err := s.sendPktLogin(); err != nil {
		return err
	}

	log.Debug("expecting login answer")
	// Example success auth packet: 0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
	//                              0xe6, 0xb2, 0x7b, 0x7b, 0xbb, 0x41, 0x3f, 0x2b,
	//                              0x00, 0x00, 0x00, 0x50, 0x02, 0x00, 0x00, 0x00,
	//                              0x00, 0x00, 0x5d, 0x37, 0x12, 0x82, 0x3b, 0xde,
	//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                              0x46, 0x54, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00,
	//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                              0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
	r, err := s.common.expect(96, []byte{0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00})
	if err != nil {
		return err
	}
	if bytes.Equal(r[48:52], []byte{0xff, 0xff, 0xff, 0xfe}) {
		return errors.New("invalid user/password")
	}

	copy(s.authID[:], r[26:32])
	s.gotAuthID = true
	if err := s.sendPktAuth(0x02); err != nil {
		return err
	}
	log.Debug("login ok, first auth sent...")

	s.requestSerialAndAudioTimeout = time.AfterFunc(5*time.Second, func() {
		reportError(errors.New("login/serial/audio request timeout"))
	})

	s.deinitNeededChan = make(chan bool)
	s.deinitFinishedChan = make(chan bool)
	go s.loop()
	return nil
}

func (s *controlStream) deinit() {
	s.deinitializing = true
	s.serialAndAudioStreamOpened = false

	if s.deinitNeededChan != nil {
		s.deinitNeededChan <- true
		<-s.deinitFinishedChan
	}
	if s.requestSerialAndAudioTimeout != nil {
		s.requestSerialAndAudioTimeout.Stop()
		s.requestSerialAndAudioTimeout = nil
	}

	if s.gotAuthID && s.common.gotRemoteSID && s.common.conn != nil {
		log.Debug("sending deauth")
		_ = s.sendPktAuth(0x01)
	}

	s.common.deinit()
	s.serial.deinit()
	s.audio.deinit()
}
