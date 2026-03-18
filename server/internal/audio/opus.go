package audio

import (
	"github.com/godeps/opus"
)

const (
	SampleRate = 48000
	Channels   = 1
	FrameSize  = 960 // 20ms at 48kHz
)

type OpusDecoder struct {
	dec *opus.Decoder
}

func NewOpusDecoder() (*OpusDecoder, error) {
	dec, err := opus.NewDecoder(SampleRate, Channels)
	if err != nil {
		return nil, err
	}
	return &OpusDecoder{dec: dec}, nil
}

// Decode decodes an Opus packet into PCM int16 samples.
func (d *OpusDecoder) Decode(data []byte) ([]int16, error) {
	pcm := make([]int16, FrameSize)
	n, err := d.dec.Decode(data, pcm)
	if err != nil {
		return nil, err
	}
	return pcm[:n], nil
}

type OpusEncoder struct {
	enc *opus.Encoder
}

func NewOpusEncoder() (*OpusEncoder, error) {
	enc, err := opus.NewEncoder(SampleRate, Channels, opus.AppVoIP)
	if err != nil {
		return nil, err
	}
	if err := enc.SetBitrate(32000); err != nil {
		return nil, err
	}
	return &OpusEncoder{enc: enc}, nil
}

// Encode encodes PCM int16 samples into an Opus packet.
func (e *OpusEncoder) Encode(pcm []int16) ([]byte, error) {
	buf := make([]byte, 1024)
	n, err := e.enc.Encode(pcm, buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}
