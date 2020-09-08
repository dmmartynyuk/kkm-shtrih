package main

import (
	"errors"
	"kkm-shtrih/drv"
	"sync"
	"time"
)

//Serv конфигурация сервера
type Serv struct {
	mu sync.Mutex
	//Config настройки сервера
	Config map[string]string
	//Drv настройки кассы
	Drv map[string]*drv.KkmDrv
}

//SetConf установка значений конфигурации сервера
func (k *Serv) SetConf(key, val string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Config[key] = val
}

//GetConf получение значений конфигурации
func (k *Serv) GetConf(key string) (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	val, ok := k.Config[key]
	return val, ok
}

//New установка нового драйвера
func (k *Serv) New(key string) *drv.KkmDrv {
	k.mu.Lock()
	defer k.mu.Unlock()
	d := drv.KkmDrv{}
	d.DeviceID = key
	d.Connected = false
	copy(d.AdminPassword[:], ADMINPASSWORD)
	copy(d.Password[:], DEFAULTPASSWORD)
	d.MaxAttemp = MAXATTEMPT
	d.TimeOut = PORTTIMEOUT
	d.Opt.Baud = int(DEFAULTBOD)
	d.Opt.Name = DEFAULTPORT
	d.Opt.ReadTimeout = time.Duration(BYTETIMEOUT) * time.Millisecond
	k.Drv[key] = &d
	return &d
}

//Add добавление нового драйвера на сервер
func (k *Serv) Add(key string, kkm *drv.KkmDrv) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Drv[key] = kkm
	return
}

/*
func (k *Serv) SetDrv(key string, val drv.KkmDrv) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Config[key]=val
}
func (k *Serv) New() (string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	val:= drv.KkmDrv{}
	return val
}
*/

//GetDrv получение драйвера
func (k *Serv) GetDrv(key string) (*drv.KkmDrv, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	val, ok := k.Drv[key]
	if !ok {
		return nil, errors.New("DeviceId " + key + " не зарегистрирован")
	}
	return val, nil
}

//GetStatusDrv получение статуса драйвера занят/не занят
func (k *Serv) GetStatusDrv(key string) (*drv.KkmDrv, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	val, ok := k.Drv[key]
	return val, ok
}

//SetStatusDrv установка статуса занятости драйвера
func (k *Serv) SetStatusDrv(key string, status bool) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	val, ok := k.Drv[key]
	if !ok {
		return errors.New("DeviceId " + key + " не зарегистрирован")
	}
	val.Busy = status
	return nil
}
