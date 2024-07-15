package telebot

import (
	"PriceWatcher/internal/entities/telebot"
	"PriceWatcher/internal/infrastructure/config"
	"context"
	"fmt"
	"time"

	tgbot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

type Telebot struct {
	bot *tgbot.BotAPI
}

func NewTelebot(configer config.Configer) (Telebot, error) {
	config, err := configer.GetConfig()
	if err != nil {
		var zero Telebot

		return zero, fmt.Errorf("can not get the config data: %v", err)
	}

	botApi, err := tgbot.NewBotAPI(config.BotKey)
	if err != nil {
		var zero Telebot
		return zero, fmt.Errorf("getting an error at connecting to the bot: %v", err)
	}

	return Telebot{bot: botApi}, nil
}

func (t Telebot) Start(ctx context.Context,
	commands []telebot.Command) error {
	updConfig := tgbot.NewUpdate(0)
	go func() {
		updCh := t.bot.GetUpdatesChan(updConfig)
		t.watchUpdates(ctx, updCh, commands)
	}()

	return nil
}

func (t Telebot) RegisterCommands(commands []telebot.Command) error {
	if err := t.configureCommands(commands); err != nil {
		return fmt.Errorf("getting an error at registering commands: %v", err)
	}

	return nil
}

func (t Telebot) Stop() {
	t.bot.StopReceivingUpdates()
}

func (t Telebot) watchUpdates(ctx context.Context,
	updCh tgbot.UpdatesChannel,
	commands []telebot.Command) {
	for {
		select {
		case upd := <-updCh:
			if upd.Message == nil {
				continue
			}

			if !upd.Message.IsCommand() {
				continue
			}

			for _, command := range commands {
				if upd.Message.Text == command.Name {
					chatID := upd.Message.Chat.ID
					comResult := command.Action(upd)

					if err := t.sendCurrentPrice(comResult, chatID); err != nil {
						logrus.Errorf("cannot send a message: %v", err)
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (t Telebot) sendCurrentPrice(msg string, chatID int64) error {
	tgMsg := tgbot.NewMessage(chatID, msg)

	maxRetries := 10
	cnt := 0
	var err error

	for cnt < maxRetries {
		if _, err = t.bot.Send(tgMsg); err != nil {
			time.Sleep(5 * time.Second)
			cnt++

			continue
		}
	}

	return err
}