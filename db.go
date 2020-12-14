package main

import (
	"errors"
	"kkm-shtrih/drv"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
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
	if len(k.Drv) == 0 {
		k.Drv = make(map[string]*drv.KkmDrv)
		k.Config = make(map[string]string)
	}
	d := drv.KkmDrv{}
	d.DeviceID = key
	d.Name = "новая kkm"
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
	if len(k.Drv) == 0 {
		k.Drv = make(map[string]*drv.KkmDrv)
		k.Config = make(map[string]string)
	}
	k.Drv[key] = kkm
	return
}

/*
func (k *Serv) SetDrv(kkm *drv.KkmDrv) {
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

//ReadDrvServ читает настройки драйвера из базы
func (k *Serv) ReadDrvServ(deviceid string) error {
	k.Drv = make(map[string]*drv.KkmDrv)
	//читаем параметры сервера
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Drivers"))
		v := b.Get([]byte(deviceid))
		if v != nil {
			kkm, err := drv.UnSerialize(v)
			if err != nil {
				return err
			}
			k.Add(string(deviceid), kkm)
			return nil
		}
		return nil
	})

	return err

}

//ReadServ читает настройки сервера из базы
func (k *Serv) ReadServ() error {
	k.Drv = make(map[string]*drv.KkmDrv)
	k.Config = make(map[string]string)
	//читаем параметры сервера
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Drivers"))
		b.ForEach(func(key, v []byte) error {
			kkm, err := drv.UnSerialize(v)
			if err != nil {
				return err
			}
			k.Add(string(key), kkm)
			return nil
		})
		return nil
	})
	if err != nil {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	err = DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("DefaultConfig"))
		b.ForEach(func(key, v []byte) error {
			k.Config[string(key)] = string(v)
			return nil
		})
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

//SetServ устанавливает настройки сервера и пишет в базу
func (k *Serv) SetServ(jkkm *drv.KkmDrvSer) error {
	d, err := k.GetDrv(jkkm.DeviceID)
	if err != nil {
		//нет такого девайса, создадим новый?
		d = k.New(jkkm.DeviceID)
	}
	k.mu.Lock()
	defer k.mu.Unlock()

	d.SetDataFromStruct(jkkm)

	//сохраним в базу
	return DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("Drivers"))
		if err != nil {
			return err
		}
		v, err := d.Serialize()
		if err != nil {
			return err
		}
		err = b.Put([]byte(d.DeviceID), v)
		if err != nil {
			return err
		}
		return nil
	})

}
