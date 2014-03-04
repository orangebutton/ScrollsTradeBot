package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
)

type Request map[string]interface{}

func SendRequest(con net.Conn, req Request) bool {
	reqMarshaled, err := json.Marshal(req)
	deny(err)

	_, err = con.Write(reqMarshaled)
	if err != nil {
		return false
	}
	return true
}

func ListenTo(url string) (net.Conn, chan []byte) {
	con, err := net.Dial("tcp", url)
	deny(err)
	ch := make(chan []byte)

	go func() {
		var replyBuffer bytes.Buffer
		readBuffer := make([]byte, 1024)
		for {
			con.SetDeadline(time.Now().Add(time.Minute))
			bytesRead, err := con.Read(readBuffer)
			if err != nil {
				close(ch)
				log.Printf("ListenTo connection error: %s", err)
				return
			}
			replyBuffer.Write(readBuffer[:bytesRead])

			lines := bytes.SplitAfter(replyBuffer.Bytes(), []byte("\n"))
			for _, line := range lines[:len(lines)-1] {
				n := len(line)
				if n > 1 {
					lineCopy := make([]byte, n)
					copy(lineCopy, line)
					ch <- lineCopy
				}
				replyBuffer.Next(n)
			}
		}
	}()

	return con, ch
}

func getLobbyURL() string {
	con, ch := ListenTo("107.21.58.31:8081")
	defer con.Close()
	SendRequest(con, Request{"msg": "LobbyLookup"})

	for reply := range ch {
		var v MLobbyLookup
		json.Unmarshal(reply, &v)
		if v.Msg == "LobbyLookup" {
			return v.Ip + ":" + strconv.Itoa(v.Port)
		}
	}

	return ""
}

func getLoginToken(email, password string) Request {
	req := Request{
		"agent": Request{
			"name":    "Scrolls",
			"version": 1,
		},
		"username": email,
		"password": password,
	}

	reqMarshaled, err := json.Marshal(req)
	deny(err)

	buf := bytes.NewBufferString(string(reqMarshaled))

	resp, err := http.Post("https://authserver.mojang.com/authenticate", "application/json", buf)
	deny(err)
	defer resp.Body.Close()

	readBuf := make([]byte, 2000)

	bytesRead, err := resp.Body.Read(readBuf)
	deny(err)

	var reply Request
	err = json.Unmarshal(readBuf[:bytesRead], &reply)
	deny(err)

	return reply
}

func Connect(email, password string) (*State, chan bool) {
	con, ch := ListenTo(getLobbyURL())
	chAlive := make(chan bool, 1)

	SendRequest(con, Request{
		"msg":         "FirstConnect",
		"accessToken": getLoginToken(email, password),
	})

	state := InitState(con)

	go func() {
		defer con.Close()
		ping := time.Tick(time.Second * 12)
		for {
			select {
			case <-state.chQuit:
				log.Printf("QUIT Connect")
				state.chQuit <- true
				return
			case req := <-state.chRequests:
				if !SendRequest(con, req) {
					state.chQuit <- true
				}
			case <-ping:
				state.SendRequest(Request{"msg": "Ping"})
			case reply := <-ch:
				if state.HandleReply(reply) {
					chAlive <- true
				} else {
					state.chQuit <- true
					return
				}
			}
		}
	}()

	return state, chAlive
}
