package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type State struct {
	Player              Player
	Gold                int
	CurrentTradeRoom    Channel
	CurrentTradePartner Player

	con           net.Conn
	chQuit        chan bool
	chMessages    chan Message
	chListeners   chan Listener
	chTradeStatus chan TradeStatus
}

type Player string
type Channel string
type Listener chan Message
type CardId int
type Message struct {
	Text    string
	From    Player
	Channel Channel
}
type TradeStatus struct {
	Partner Player
	Updated bool
	Their   struct {
		Cards    []string
		Gold     int
		Accepted bool
	}
	My struct {
		Cards    []string
		Gold     int
		Accepted bool
	}
}

var (
	CardTypes    = make(map[CardId]string)
	CardRarities = make(map[string]int)
	Libraries    = make(map[Player]MLibraryView)
	Stocks       = make(map[Player]map[string]int)
	PlayersIds   = make(map[Player]string)
)

func InitState(con net.Conn) *State {
	s := State{con: con}
	s.chQuit = make(chan bool, 1)
	s.chMessages = make(chan Message)
	s.chListeners = make(chan Listener)
	s.chTradeStatus = make(chan TradeStatus)
	s.SendRequest(Request{"msg": "JoinLobby"})
	s.SendRequest(Request{"msg": "LibraryView"})

	go func() {
		listeners := make([]Listener, 0)

		for {
			select {
			case <-s.chQuit:
				s.chQuit <- true
				return
			case newListener := <-s.chListeners:
				listeners = append(listeners, newListener)
			case m := <-s.chMessages:
				for i, l := range listeners {
					select {
					case l <- m:
					default:
						if i < len(listeners)-1 {
							copy(listeners[i:], listeners[i+1:])
						}
						listeners = listeners[:len(listeners)-1]
					}
				}
			}
		}
	}()

	return &s
}

func (s *State) SendRequest(req Request) {
	logMessage("-> " + fmt.Sprintf("%s\n", req))
	SendRequest(s.con, req)
}

func (s *State) Listen() Listener {
	l := make(Listener, 10)
	s.chListeners <- l
	return l
}

func (s *State) JoinRoom(room Channel) {
	s.SendRequest(Request{"msg": "RoomEnter", "roomName": room})
	for m := range s.Listen() {
		if m.Channel == room {
			return
		}
	}
}

func (s *State) LeaveRoom(room Channel) {
	s.SendRequest(Request{"msg": "RoomExit", "roomName": room})
}

func (s *State) Say(room Channel, text string) {
	s.SendRequest(Request{"msg": "RoomChatMessage", "text": text, "roomName": room})
	timeout := time.After(2 * time.Second)
	for {
		select {
		case m := <-s.Listen():
			if m.Channel == room && m.From == s.Player && m.Text == text {
				return
			}

			if m.From == "Scrolls" && m.Text == "You have been temporarily muted (for flooding the chat or by a moderator)." {
				time.Sleep(time.Second)
				s.SendRequest(Request{"msg": "RoomChatMessage", "text": text, "roomName": room})
			}
		case <-timeout:
			logMessage("!!!MESSAGE TIMEOUT!!!")
			return
		}
	}
}

func (s *State) Whisper(player Player, text string) {
	s.SendRequest(Request{"msg": "Whisper", "text": text, "toProfileName": player})
}

func (s *State) InitiateTrade(player Player, timeout time.Duration) chan TradeStatus {
	id := PlayersIds[player]

	s.CurrentTradePartner = ""
	s.SendRequest(Request{"msg": "TradeInvite", "profile": id})

	cancel := time.After(timeout)
	for {
		select {
		case <-s.chTradeStatus:
			if s.CurrentTradePartner == "" { // they rejected the trade invite
				return nil
			}
		case m := <-s.Listen(): // find out what room we're trading in
			if strings.HasPrefix(string(m.Channel), "trade-") {
				s.CurrentTradeRoom = m.Channel
				return s.chTradeStatus
			}
		case <-cancel:
			// Todo: what happens if the player accepts after timeout?
			return nil
		}
	}
}

func (s *State) HandleReply(reply []byte) bool {
	if len(reply) < 2 {
		logMessage("reply is too short\n")
		return false
	}

	var m Reply
	err := json.Unmarshal(reply, &m)
	if err != nil {
		logMessage(fmt.Sprintf("%s\n", err))
		return false
	}

	if m.Msg != "AvatarTypes" &&
		m.Msg != "CardTypes" &&
		m.Msg != "AchievementTypes" &&
		m.Msg != "LibraryView" {
		logMessage("<- " + string(reply))
	}

	switch m.Msg {
	case "AchievementUnlocked":
		var v MAchievementUnlocked
		json.Unmarshal(reply, &v)

	case "AchievementTypes":
		var v MAchievementTypes
		json.Unmarshal(reply, &v)

	case "ActiveGame":
		var v MActiveGame
		json.Unmarshal(reply, &v)

	case "AvatarTypes":
		var v MAvatarTypes
		json.Unmarshal(reply, &v)

	case "CardTypes":
		var v MCardTypes
		json.Unmarshal(reply, &v)
		for _, cardType := range v.CardTypes {
			CardTypes[CardId(cardType.Id)] = cardType.Name
			CardRarities[cardType.Name] = cardType.Rarity

			var lowerPrice, upperPrice int
			switch cardType.Rarity {
			case 0:
				lowerPrice = 50
				upperPrice = 150
			case 1:
				lowerPrice = 50
				upperPrice = 500
			case 2:
				lowerPrice = 50
				upperPrice = 1500
			}

			price := Prices[cardType.Name]
			if price.Buy < lowerPrice {
				price.Buy = lowerPrice
			}
			if price.Sell < lowerPrice {
				price.Sell = lowerPrice
			}
			if price.Sell > upperPrice {
				price.Sell = upperPrice
			}
			if price.Buy > upperPrice {
				price.Buy = upperPrice
			}

			suggPrice := (price.Buy + price.Sell) / 2
			price.Buy = suggPrice * 80 / 100
			if cardType.Rarity == 2 {
				price.Buy = suggPrice
			}

			price.Sell = suggPrice * 110 / 100
			Prices[cardType.Name] = price
		}

	case "Fail":
		var v MFail
		json.Unmarshal(reply, &v)
		if v.Op != "Whisper" {
			s.Whisper("redefiance", fmt.Sprintf("%s", v))
		}
		fmt.Println(v)
		// return false

	case "FatalFail":
		var v MFatalFail
		json.Unmarshal(reply, &v)
		fmt.Println(v)
		return false

	case "GetBlockedPersons":
		var v MGetBlockedPersons
		json.Unmarshal(reply, &v)

	case "GetFriendRequests":
		var v MGetFriendRequests
		json.Unmarshal(reply, &v)

	case "GetFriends":
		var v MGetFriends
		json.Unmarshal(reply, &v)

	case "LibraryView":
		var v MLibraryView
		json.Unmarshal(reply, &v)

		var player Player
		for playerName, id := range PlayersIds {
			if id == v.ProfileId {
				player = playerName
				break
			}
		}

		Libraries[player] = v
		stock := make(map[string]int)
		for _, card := range CardTypes {
			stock[card] = 0
		}

		for _, card := range v.Cards {
			if card.Tradable {
				name := CardTypes[CardId(card.TypeId)]
				stock[name]++
			}
		}
		Stocks[player] = stock

	case "Ok":
		var v MOk
		json.Unmarshal(reply, &v)

	case "Ping":
		var v MPing
		json.Unmarshal(reply, &v)

	case "ProfileDataInfo":
		var v MProfileDataInfo
		json.Unmarshal(reply, &v)
		s.Gold = v.ProfileData.Gold

	case "ProfileInfo":
		var v MProfileInfo
		json.Unmarshal(reply, &v)
		s.Player = Player(v.Profile.Name)

	case "RoomChatMessage":
		var v MRoomChatMessage
		json.Unmarshal(reply, &v)
		// if Player(v.From) != s.Player {
		s.chMessages <- Message{v.Text, Player(v.From), Channel(v.RoomName)}
		// }

	case "RoomEnter":
		var v MRoomEnter
		json.Unmarshal(reply, &v)

	case "RoomInfo":
		var v MRoomInfo
		json.Unmarshal(reply, &v)
		for _, player := range v.Updated {
			PlayersIds[Player(player.Name)] = player.Id
		}
	case "ServerInfo":
		var v MServerInfo
		json.Unmarshal(reply, &v)

	case "TradeResponse":
		var v MTradeResponse
		json.Unmarshal(reply, &v)
		if v.Status == "DECLINE" {
			s.chTradeStatus <- TradeStatus{}
		} else {
			if Player(v.From.Name) == s.Player {
				s.CurrentTradePartner = Player(v.To.Name)
			} else {
				s.CurrentTradePartner = Player(v.From.Name)
			}
		}
		s.chTradeStatus <- TradeStatus{}

	case "TradeView":
		var v MTradeView
		json.Unmarshal(reply, &v)

		my := v.From
		their := v.To
		if their.Profile.Id == PlayersIds[s.Player] {
			my, their = their, my
		}

		convertIdsToNames := func(cardIds []int, player Player) []string {
			names := make([]string, len(cardIds))
			for i, id := range cardIds {
				for _, card := range Libraries[player].Cards {
					if card.Id == id {
						names[i] = CardTypes[CardId(card.TypeId)]
						break
					}
				}
			}
			return names
		}

		ts := TradeStatus{}
		ts.Partner = s.CurrentTradePartner
		ts.Updated = v.Modified
		ts.Their.Accepted = their.Accepted
		ts.Their.Cards = convertIdsToNames(their.CardIds, s.CurrentTradePartner)
		ts.Their.Gold = their.Gold
		ts.My.Accepted = my.Accepted
		ts.My.Cards = convertIdsToNames(my.CardIds, s.Player)
		ts.My.Gold = my.Gold

		s.chTradeStatus <- ts

	case "Whisper":
		var v MWhisper
		json.Unmarshal(reply, &v)
		if Player(v.From) != s.Player {
			s.chMessages <- Message{v.Text, Player(v.From), Channel("WHISPER")}
		}

	default:
		fmt.Println(string(reply))
	}

	return true
}

func logMessage(s string) {
	file, err := os.OpenFile("/home/stargazer/go/src/ScrollsTradeBot/log.txt", os.O_WRONLY+os.O_APPEND, 0)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	io.WriteString(file, s)
}
