package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wurt83ow/timetracker/internal/models"
	"go.uber.org/zap"
)

type ExtController struct {
	ctx     context.Context
	storage Storage
	log     Log
	extAddr func() string
}

type Pool interface {
	AddResults(interface{})
	GetResults() <-chan interface{}
}

func NewExtController(ctx context.Context, storage Storage, extAddr func() string, log Log) *ExtController {
	return &ExtController{
		ctx:     ctx,
		storage: storage,
		log:     log,
		extAddr: extAddr,
	}
}

func (c *ExtController) GetUserInfo(passportSerie int, passportNumber int) (models.ExtUserData, error) {

	addr := c.extAddr()

	// Добавление http схемы если она отсутствует
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}

	if string(addr[len(addr)-1]) != "/" {
		addr = addr + "/"
	}

	url := fmt.Sprintf("%sinfo?passportSerie=%d&passportNumber=%d", addr, passportSerie, passportNumber)

	fmt.Println(url)
	resp, err := http.Get(url)
	if err != nil {
		c.log.Info("unable to access user info service, check that it is running: ", zap.Error(err))
		return models.ExtUserData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.Info("status code error: ", zap.String("method", resp.Status))
		return models.ExtUserData{}, fmt.Errorf("status code error: %s", resp.Status)
	}

	var userInfo models.ExtUserData
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return models.ExtUserData{}, err
	}

	userInfo.PassportSerie = passportSerie
	userInfo.PassportNumber = passportNumber

	return userInfo, nil
}
