package main

import (
	"log"
	"os"
	"regexp"
	"time"
)

var reNumbers = regexp.MustCompile(`x?(\d+)x?`)
var reInvalidChars = regexp.MustCompile("[^a-z'0-9 ]")

var WTBrequests = make(map[Player]map[Card]int)
var Bot Player
var currentState *State

const MyRoom = "autobots" // you will need to change the room

func main() {
	log.Print("main start...")

	config := LoadConfig()

	if config.Log {
		f, err := os.OpenFile("system.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		deny(err)
		log.SetOutput(f)
		//log.SetOutput(ioutil.Discard)
	}

	if config.UseWebserver {
		go startWebServer()
	}

	for {
		startBot(config.Email, config.Password, "Hello world!")
	}
}

func deny(err error) {
	if err != nil {
		panic(err)
	}
}

func startBot(email, password, helloMessage string) {

	s, chAlive := Connect(email, password)
	currentState = s

	s.JoinRoom(MyRoom)
	//s.JoinRoom("trading-1")
	s.JoinRoom("trading-2")

	if helloMessage != "" {
		s.Say(MyRoom, helloMessage)
		s.Say("trading-1", helloMessage)
	}

	//upSince := time.Now()
	chKillThread := make(chan bool, 1)

	go func() {
		queue := make([]Player, 0)
		chReadyToTrade := make(chan bool, 100)

		messages := s.Listen()
		defer s.Shut(messages)
		for {
			select {
			case <-chKillThread:
				return
			case <-chReadyToTrade:
				go func() {
					s.handlePreTrade(queue)
					stockBefore := Stocks[Bot]
					if stockBefore == nil {
						stockBefore = make(map[Card]int)
					}
					tradeStatus := s.Trade(queue[0])
					s.handlePostTrade(stockBefore, tradeStatus)
					queue = queue[1:]
					if len(queue) == 0 {
						s.Say(MyRoom, "Finished trading.")
					} else {
						chReadyToTrade <- true
					}
				}()

			case m := <-messages:
				queue = s.HandleMessages(m, queue, chReadyToTrade)
			}
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered from: %s", r)
		}

		log.Print("killing thread")
		chKillThread <- true
		log.Print("Restarting..")
	}()

	for {
		timeout := time.After(time.Minute * 1)
	InnerLoop:
		for {
			select {
			case <-chAlive:
				break InnerLoop
			case <-s.chQuit:
				log.Println("!!!main QUIT!!!")
				s.chQuit <- true
				return
			case <-timeout:
				log.Println("!!!TIMEOUT!!!")
				return
			}
		}
	}
}
