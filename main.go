package main

import (
	"log"
	"os"
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

	//upSince := time.Now()
	chKillThread := make(chan bool, 1)
	queue := make(chan Player, 100)

	s.joinRoomsAndSayHi()
	s.startTradeThread(queue)
	s.startMessageHandlingThread(queue, chKillThread)

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

func (s *State) startTradeThread(queue <-chan Player) {

	go func() {
		for {
			player := <-queue
			s.Trade(player)
			if len(queue) == 0 {
				s.Say(Conf.Room, "Finished trading.")
			}
		}
	}()
}

func (s *State) startMessageHandlingThread(queue chan<- Player, chKillThread chan bool) {
	go func() {
		messages := s.Listen()
		defer s.Shut(messages)
		for {
			select {
			case <-chKillThread:
				return
			case m := <-messages:
				s.HandleMessages(m, queue)
			}
		}
	}()
}

func (s *State) joinRoomsAndSayHi() {
	s.JoinRoom(Conf.Room)
	s.JoinRoom("trading-1")
	s.JoinRoom("trading-2")

	if HelloMessage != "" {
		s.Say(Conf.Room, HelloMessage)
		//	s.Say("trading-1", HelloMessage)
		//	s.Say("trading-2", HelloMessage)
	}
}

func deny(err error) {
	if err != nil {
		panic(err)
	}
}
