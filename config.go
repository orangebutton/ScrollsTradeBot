package main

import (
	"encoding/json"
	"log"
	"os"
)

/*
{
	"Email": "foo@foo.com",
	"Password": "mypassword",
	"Owner": "redefiance",
	"Room": "clockwork",

	"GoldDivisor": 5,
	"GoldThreshold": 2000.0,
	"MaxNumToBuy": 12.0,

	"UseScrollsGuidePrice": false,
	"UseWebserver": false,
	"Log": true
}
*/

type Config struct {
	Email    string
	Password string
	Owner    Player
	Room     Channel

	GoldDivisor          int
	GoldThreshold        float64
	MaxNumToBuy          float64
	UseScrollsGuidePrice bool

	UseWebserver bool
	Log          bool
}

func LoadConfig() *Config {
	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	config := new(Config)
	decoder.Decode(config)

	log.Println("config ", config)

	log.Println("Email: ", config.Email)
	log.Println("Room: ", config.Room)
	log.Println("Owner: ", config.Owner)

	log.Println("GoldDivisor: ", config.GoldDivisor)
	log.Println("GoldThreshold: ", config.GoldThreshold)
	log.Println("MaxNumToBuy: ", config.MaxNumToBuy)
	log.Println("UseScrollsGuidePrice: ", config.UseScrollsGuidePrice)

	log.Println("UseWebserver: ", config.UseWebserver)
	log.Println("Log: ", config.Log)

	return config
}
