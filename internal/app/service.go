package app

import (
	"PriceWatcher/internal/app/clock"
	"PriceWatcher/internal/app/interrupt"
	"PriceWatcher/internal/domain/message"
	"PriceWatcher/internal/domain/price/analyser"
	"PriceWatcher/internal/domain/price/extractor"
	"PriceWatcher/internal/interfaces/configer"
	"PriceWatcher/internal/interfaces/file"
	interReq "PriceWatcher/internal/interfaces/requester"
	interSend "PriceWatcher/internal/interfaces/sender"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

type PriceService struct {
	req      interReq.Requester
	sender   interSend.Sender
	ext      extractor.Extractor
	val      message.HourValidator
	analyser analyser.Analyser
	wr       file.WriteReader
	conf     configer.Configer
}

var bankUrl = "https://investzoloto.ru/gold-sber-oms/"

func (s *PriceService) serve(clock clock.Clock) error {
	curHour := clock.Now().Hour()

	conf, err := s.conf.GetConfig()
	if err != nil {
		return fmt.Errorf("on getting the config an error occurs: %w", err)
	}

	logrus.Infof("Check time for processing a price. The time value: %v", curHour)

	if s.analyser == nil && !s.val.Validate(curHour, conf.SendingHours) {
		logrus.Info("It is not appropriate time for getting a price")

		return nil
	}

	logrus.Info("Start processing a price")

	itemPrices := conf.Items

	if s.analyser != nil {
		curPrices, err := s.wr.Read()
		if err != nil {
			return err
		}

		crossedKeys := make([]string, len(curPrices))

		for k := range curPrices {
			if _, ok := itemPrices[k]; ok {
				crossedKeys = append(crossedKeys, k)

				continue
			}

			delete(curPrices, k)
		}

		for k := range itemPrices {
			if !slices.Contains(crossedKeys, k) {
				curPrices[k] = 0.0
			}
		}

		for k, v := range itemPrices {

			response, err := s.req.RequestPage(v)
			if err != nil {
				return fmt.Errorf("cannot get a page with the current price: %w", err)
			}

			price, err := s.ext.ExtractPrice(response.Body)
			if err != nil {
				return fmt.Errorf("cannot extract the price from the body: %w", err)
			}

			changed, up, amount := s.analyser.AnalysePrice(price, float32(curPrices[k]))

			if changed && !up {
				sub := "Цена на товар WB"
				msg := fmt.Sprintf("Цена на %v уменьшилась на %.2fр. Текущая цена: %.2fр", itemPrices[k], amount, price)

				err := s.sender.Send(msg, sub, conf.Email)
				if err != nil {
					return fmt.Errorf("cannot send the item price: %w", err)
				}

				curPrices[k] = float64(price)

				logrus.Info("The item price has been changed. A report is sended")

				continue
			}

			if curPrices[k] == 0.0 {
				curPrices[k] = float64(price)
			}

			logrus.Info("The item price has been not changed")

			if len(itemPrices) > 1 {
				dur := time.Duration(60+rand.Intn(120)) * time.Second
				time.Sleep(dur)
			}
		}

		err = s.wr.Write(curPrices)
		if err != nil {
			return err
		}

		return nil
	}

	response, err := s.req.RequestPage(bankUrl)
	if err != nil {
		return fmt.Errorf("cannot get a page with the current price: %w", err)
	}

	price, err := s.ext.ExtractPrice(response.Body)
	if err != nil {
		return fmt.Errorf("cannot extract the price from the body: %w", err)
	}

	msg := fmt.Sprintf("Курс золота. Продажа: %.2fр", price)
	sub := "Че по золоту?"

	err = s.sender.Send(msg, sub, conf.Email)
	if err != nil {
		return fmt.Errorf("cannot send the price: %w", err)
	}

	logrus.Info("The price is processed")

	return nil
}

func (s *PriceService) Watch(done <-chan struct{}, cancel context.CancelFunc, clock clock.Clock) {
	interrupt.WatchForInterruption(cancel)

	errMes := "An error occurs while serving a price: %v"
	if err := s.serve(clock); err != nil {
		logrus.Errorf(errMes, err)
	}

	config, err := s.conf.GetConfig()
	if err != nil {
		logrus.Errorf("An error occurs while get the config data: %v", err)

		return
	}

	if strings.ToLower(config.PriceType) == "bank" {
		err := waitHourStart(clock.Now())
		if err != nil {
			logrus.Errorf("An error occurs while waiting when the next hour begins: %v", err)

			return
		}
	}

	var dur time.Duration

	if strings.ToLower(config.PriceType) == "marketplace" {
		dur = time.Duration(20 * time.Minute)
	} else {
		dur = time.Duration(1 * time.Hour)
	}

	t := time.NewTicker(dur)
	defer t.Stop()

	for {
		select {
		case <-done:
			logrus.Info("Shut down the application")
			return
		case <-t.C:
			if err := s.serve(clock); err != nil {
				logrus.Errorf(errMes, err)
			}

			if strings.ToLower(config.PriceType) == "marketplace" {
				dur = time.Duration(20+rand.Intn(10)) * time.Minute
				t.Reset(dur)
			}
		}
	}
}
