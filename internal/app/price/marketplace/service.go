package marketplace

import (
	"PriceWatcher/internal/domain/price/analyser"
	"PriceWatcher/internal/domain/price/extractor"
	"PriceWatcher/internal/entities/config"
	"PriceWatcher/internal/interfaces/file"
	"PriceWatcher/internal/interfaces/requester"
	"fmt"
	"math/rand"
	"strings"
	"time"
	"unicode"

	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

type Service struct {
	wr       file.WriteReader
	req      requester.Requester
	ext      extractor.Extractor
	analyser analyser.Analyser
}

func NewService(
	wr file.WriteReader,
	req requester.Requester,
	ext extractor.Extractor,
	analyser analyser.Analyser) Service {

	return Service{
		wr:       wr,
		req:      req,
		ext:      ext,
		analyser: analyser,
	}
}

func (s Service) ServePrice(conf config.Config) (message, subject string, err error) {

	itemPrices := conf.Items

	curPrices, err := s.wr.Read()
	if err != nil {
		return "", "", err
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

	priceType := capitalize(conf.PriceType)
	sub := fmt.Sprintf("Цена на товар %v", priceType)
	messages := make([]string, 0)

	for k, v := range itemPrices {

		response, err := s.req.RequestPage(v)
		if err != nil {
			return "", "", fmt.Errorf("cannot get a page with the current price: %w", err)
		}

		price, err := s.ext.ExtractPrice(response.Body)
		if err != nil {
			return "", "", fmt.Errorf("cannot extract the price from the body: %w", err)
		}

		changed, up, amount := s.analyser.AnalysePrice(price, float32(curPrices[k]))

		if changed && !up {
			msg := fmt.Sprintf("Цена на %v %v уменьшилась на %.2fр. Текущая цена: %.2fр\n", k, itemPrices[k], amount, price)
			messages = append(messages, msg)

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
		return "", "", err
	}

	return strings.Join(messages, "\n"), sub, nil
}

func capitalize(str string) string {
	runes := []rune(str)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func (Service) GetWaitTime() time.Duration {
	return getWaitTime()
}

func (Service) WaitNextStart(now time.Time) (time.Duration, error) {
	return waitNextStart(now)
}
