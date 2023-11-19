package config

type Config struct {
	Services []ServiceConf
}

type ServiceConf struct {
	SendingHours []int
	PriceType    string
	Marketplace  string
	Items        map[string]string
	Email        Email
}

type Email struct {
	From     string
	Pass     string
	To       string
	SmtpHost string
	SmtpPort int
}
