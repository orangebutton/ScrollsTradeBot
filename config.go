package main

import (
	"encoding/json"
	"log"
	"os"
)

//{
//    "Email": "foo@foo.com",
//    "Password": "mypassword",
//    "UseWebserver": false,
//    "Log": true
//}
type Config struct {
	Email        string
	Password     string
	UseWebserver bool
	Log          bool
}

func LoadConfig() *Config {
	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	config := new(Config)
	decoder.Decode(config)

	log.Println("Email: ", config.Email)
	log.Println("UseWebserver: ", config.UseWebserver)
	log.Println("Log: ", config.Log)

	return config
}
