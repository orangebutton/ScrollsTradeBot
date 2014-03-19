package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

//const HelloMessage = "fixed pricing"
//const HelloMessage = "wts/wtb - lots of cards - to trade with me, join room autobots and type help"

const HelloMessage = "wts/wtb - lots of cards - to trade with me, join room autobots and type help"

var WTBrequests = make(map[Player]map[Card]int)
var Bot Player
var Conf *Config
var currentState *State

func main() {
	log.Print("main start...")

	Conf = LoadConfig()

	if Conf.Log {
		f, err := os.OpenFile("system.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		deny(err)
		log.SetOutput(f)
		//log.SetOutput(ioutil.Discard)
	}

	if Conf.UseWebserver {
		go startWebServer()
	}

	for {
		startBot(Conf.Email, Conf.Password, HelloMessage)
	}
}

func startBot(email, password, helloMessage string) {

	s, chAlive := Connect(email, password)
	currentState = s

	s.JoinRoom(Conf.Room)
	s.JoinRoom("trading-1")
	s.JoinRoom("trading-2")

	if helloMessage != "" {
		s.Say(Conf.Room, helloMessage)
		s.Say("trading-1", helloMessage)
		s.Say("trading-2", helloMessage)
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
					s.sayTradingParter(queue)
					s.Trade(queue[0])
					queue = queue[1:]
					if len(queue) == 0 {
						s.Say(Conf.Room, "Finished trading.")
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

func deny(err error) {
	if err != nil {
		panic(err)
	}
}

func (s *State) sayTradingParter(queue []Player) {
	if len(queue) > 1 {
		waiting := make([]string, len(queue)-1)
		for i, name := range queue[1:] {
			waiting[i] = string(name)
		}
		s.Say(Conf.Room, fmt.Sprintf("Now trading with [%s] < %s", queue[0], strings.Join(waiting, " < ")))
	} else {
		s.Say(Conf.Room, fmt.Sprintf("Now trading with [%s].", queue[0]))
	}
}
