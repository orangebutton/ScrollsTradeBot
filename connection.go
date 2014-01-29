package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"time"
)

type Request map[string]interface{}

func SendRequest(con net.Conn, req Request) {
	reqMarshaled, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}

	_, err = con.Write(reqMarshaled)
	if err != nil {
		panic(err)
	}
}

func ListenTo(url string) (net.Conn, chan []byte) {
	con, err := net.Dial("tcp", url)
	if err != nil {
		panic(err)
	}
	ch := make(chan []byte)

	go func() {
		var replyBuffer bytes.Buffer
		readBuffer := make([]byte, 1024)
		for {
			con.SetDeadline(time.Now().Add(time.Minute))
			bytesRead, err := con.Read(readBuffer)
			if err != nil {
				close(ch)
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
	if err != nil {
		panic(err)
	}

	buf := bytes.NewBufferString(string(reqMarshaled))

	resp, err := http.Post("https://authserver.mojang.com/authenticate", "application/json", buf)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	readBuf := make([]byte, 2000)

	bytesRead, err := resp.Body.Read(readBuf)
	if err != nil {
		panic(err)
	}

	var reply Request
	err = json.Unmarshal(readBuf[:bytesRead], &reply)
	if err != nil {
		panic(err)
	}
	return reply
}

func Connect(email, password string) (*State, chan bool) {
	con, ch := ListenTo(getLobbyURL())
	chAlive := make(chan bool, 10)

	SendRequest(con, Request{
		"msg":         "FirstConnect",
		"accessToken": getLoginToken(email, password),
	})
	SendRequest(con, Request{"msg": "JoinLobby"})

	state := InitState(con)

	go func() {
		defer con.Close()
		ping := time.Tick(time.Second * 15)
		for {
			select {
			case <-ping:
				state.SendRequest(Request{"msg": "Ping"})
			case reply := <-ch:
				select {
				case chAlive <- true:
				default:
				}

				if !state.HandleReply(reply) {
					state.chQuit <- true
				}
			case <-state.chQuit:
				logMessage("QUIT")
				return
			}
		}
	}()

	return state, chAlive
}
