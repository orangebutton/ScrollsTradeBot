package main

type Reply struct {
	Msg string
}

type MActiveGame struct {
	Reply
	// empty - I have no idea what is up with that
}

type MAchievementTypes struct {
	Reply
	AchievementTypes []struct {
		Id          int
		Name        string
		Description string
		GoldReward  int
		Group       int
		SortId      int
		Icon        string
	}
}

type MAchievementUnlocked struct {
	Reply
	TypeId int
}

type MAvatarTypes struct {
	Reply
	Types []struct {
		Id       int
		Type     string
		Part     string
		Filename string
		Set      string
	}
}

type MCardTypes struct {
	Reply
	CardTypes []struct {
		Id                    int
		Name                  string
		Description           string
		SubTypesStr           string
		Kind                  string
		Rarity                int
		CostGrowth            int
		CostOrder             int
		CostEnergy            int
		CostDecay             int
		Ap                    int
		Ac                    int
		Hp                    int
		TargetArea            string
		Set                   int
		Flavor                string
		Sound                 string
		Available             bool
		AnimationPreviewImage int
		CardImage             int
		AnimationPreviewInfo  string
		AnimationBundle       int
		PassiveRules          []struct {
			DisplayName string
			Description string
		}
		RulesList []string
		Abilities []struct {
			Id          string
			Name        string
			Description string
			Cost        struct {
				Decay  int
				Order  int
				Energy int
				Growth int
			}
		}
	}
}

type MFail struct {
	Reply
	Op   string
	Info string
}

type MFatalFail struct {
	Reply
	Info string
}

type MFriendRequestUpdate struct {
	Reply
	Request struct {
		From struct {
			Profile struct {
				Id        string
				UserUuid  string
				Name      string
				AdminRole string
				UserType  string
			}
		}
		To struct {
			Profile struct {
				Id        string
				UserUuid  string
				Name      string
				AdminRole string
				UserType  string
			}
			OnlineState string
		}
		Request struct {
			Id               string
			RequestingUserId string
			TargetUserId     string
			Status           string
		}
	}
}

type MFriendUpdate struct {
	Reply
	Friend struct {
		Profile struct {
			Id        string
			UserUuid  string
			Name      string
			AdminRole string
			UserType  string
		}
		OnlineStatus string
	}
}

type MGetFriends struct {
	Reply
	Friends []struct {
		Profile struct {
			Id        string
			UserUuid  string
			Name      string
			AdminRole string
			UserType  string
		}
		OnlineState string
	}
}

type MGetFriendRequests struct {
	Reply
	Requests []struct {
		From struct {
			Profile struct {
				Id        string
				UserUuid  string
				Name      string
				AdminRole string
				UserType  string
			}
		}
		To struct {
			Profile struct {
				Id        string
				UserUuid  string
				Name      string
				AdminRole string
				UserType  string
			}
			OnlineState string
		}
		Request struct {
			Id               string
			RequestingUserId string
			TargetUserId     string
			Status           string
		}
	}
}

type MGetBlockedPersons struct {
	Reply
	// TODO
}

type MLibraryView struct {
	Reply
	ProfileId string
	Cards     []struct {
		Id       int
		TypeId   int
		Tradable bool
		isToken  bool
		Level    int
	}
}

type MLobbyLookup struct {
	Reply
	Ip   string
	Port int
}

type MOk struct {
	Reply
	Op string
}

type MPing struct {
	Reply
	Time int
}

type MProfileDataInfo struct {
	Reply
	ProfileData struct {
		Gold                   int
		Shards                 int
		Rating                 int
		SelectedPreconstructed int
		AcceptChallenges       bool
		acceptTrades           bool
		spectatePermission     string
	}
}

type MProfileInfo struct {
	Reply
	Profile struct {
		Id        string
		UserUuid  string
		Name      string
		AdminRole string
		UserType  string
	}
}

type MRoomChatMessage struct {
	Reply
	RoomName string
	From     string
	Text     string
}

type MRoomEnter struct {
	Reply
	RoomName string
}

type MRoomInfo struct {
	Reply
	RoomName string
	Reset    bool
	Updated  []struct {
		Name             string
		Id               string
		AcceptChallenges bool
		AcceptTrades     bool
		AdminRole        string
	}
	Removed []struct {
		Name string
	}
}

type MServerInfo struct {
	Reply
	Version  string
	AssetURL string
	Roles    string
}

type MTradeInviteForward struct {
	Reply
	Inviter struct {
		Id        string
		UserUuid  string
		Name      string
		AdminRole string
		UserType  string
	}
}

type MTradeResponse struct {
	Reply
	From struct {
		Id        string
		UserUuid  string
		Name      Player
		AdminRole string
		UserType  string
	}
	To struct {
		Id        string
		UserUuid  string
		Name      Player
		AdminRole string
		UserType  string
	}
	Status string
}

type MTradeView struct {
	Reply
	From struct {
		Profile struct {
			Id        string
			UserUuid  string
			Name      string
			AdminRole string
			UserType  string
		}
		CardIds  []int
		Gold     int
		Accepted bool
	}
	To struct {
		Profile struct {
			Id        string
			UserUuid  string
			Name      string
			AdminRole string
			UserType  string
		}
		CardIds  []int
		Gold     int
		Accepted bool
	}
	Modified bool
}

type MWhisper struct {
	Reply
	ToProfileName string
	From          string
	Text          string
}
