package android_server

import (
	"bufio"
	"encoding/binary"
	"image"
	"io"
	"net"
	"net/http"

	"github.com/pixiv/go-libjpeg/jpeg"
	log "github.com/sirupsen/logrus"
)

var imageChan = make(chan image.Image, 1)
var conn net.Conn

// Get images from TCP stream and add them to imageChan
func GetTCPStream(conn net.Conn, imageChan chan image.Image) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", "localhost:1313")
	if err != nil {
		log.Fatal(err)
	}
	idxReDialCnt := 0
	for {
		conn, err := net.DialTCP("tcp", nil, tcpAddr)
		if err != nil {
			if idxReDialCnt < 10 {
				idxReDialCnt += 1
				continue
			} else {
				break
			}
		}
		var pid, rw, rh, vw, vh uint32
		var version uint8
		var unused uint8
		var orientation uint8
		binRead := func(data interface{}) error {
			if err != nil {
				return err
			}
			err = binary.Read(conn, binary.LittleEndian, data)
			return err
		}
		binRead(&version)
		binRead(&unused)
		binRead(&pid)
		binRead(&rw)
		binRead(&rh)
		binRead(&vw)
		binRead(&vh)
		binRead(&orientation)
		binRead(&unused)
		if err != nil {
			continue
		}

		bufrd := bufio.NewReader(conn) // Do not put it into for loop
		for {
			var size uint32
			if err = binRead(&size); err != nil {
				log.Fatal(err)
				break
			}
			lr := &io.LimitedReader{bufrd, int64(size)}
			var im image.Image
			im, err = jpeg.Decode(lr, &jpeg.DecoderOptions{})
			if err != nil {
				break
			}

			select {
			case imageChan <- im:
			default:
			}
		}
		conn.Close()
	}
}

type StreamHandler struct {
	Next    func() (image.Image, error)
	Options *jpeg.EncoderOptions
}

func (h StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	boundary := "\r\n--frame\r\nContent-Type: image/jpeg\r\n\r\n"
	for {
		// get handler new image from imageChan
		img, err := h.Next()
		if err != nil {
			return
		}

		n, err := io.WriteString(w, boundary)
		if err != nil || n != len(boundary) {
			return
		}

		err = jpeg.Encode(w, img, h.Options)
		if err != nil {
			return
		}

		n, err = io.WriteString(w, "\r\n")
		if err != nil || n != 2 {
			return
		}
	}
}

func MinicapStreamHandler() *StreamHandler {
	// for each new image in imageChan update the handler
	stream := StreamHandler{
		Next: func() (image.Image, error) {
			return <-imageChan, nil
		},
		Options: &jpeg.EncoderOptions{Quality: 50, OptimizeCoding: true},
	}

	go GetTCPStream(conn, imageChan)

	return &stream
}