package config

import (
	"encoding/json"
	"log"
	"os"
)

type Service struct {
	Host 	string	`json:"host"`
	Port 	int		`json:"port"`
}

type Config struct {
	Port    	int					`json:"port"`
	Host		string				`json:"host"`
	Services 	map[string]Service	`json:"services"`
}

func LoadConfig(path string) Config {
	config := Config{
		Port: 8080, 
		Host: "localhost",
		Services: map[string]Service{},
	}
 	configFile, err := os.Open(path)

	if err != nil {
  		log.Fatal(err)
 	}

	defer configFile.Close()
	
 	jsonParser := json.NewDecoder(configFile)
 	jsonParser.Decode(&config)

	return config
}