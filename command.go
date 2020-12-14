package main

import (
	"encoding/binary"
	"errors"
	"kkm-shtrih/drv"
	"log"
)

func parseAnswer(insdata []byte) (command uint16, errcode byte, data []byte) {
	if insdata[0] == 0xff {
		command = binary.BigEndian.Uint16(insdata[0:2])
		errcode = insdata[2]
		if len(insdata) > 3 {
			data = insdata[3:]
		} else {
			data = []byte{}
		}
	} else {
		command = binary.BigEndian.Uint16(insdata[0:1])
		errcode = insdata[1]
		if len(insdata) > 2 {
			data = insdata[2:]
		} else {
			data = []byte{}
		}
	}
	return
}

func kkmRunFunction(k *drv.KkmDrv, fname string, param []byte) (errcode byte, data []byte, err error) {
	switch fname {
	case "beep":
		err = kkmBeep(k)
	}
	return
}

func kkmBeep(k *drv.KkmDrv) error {
	res, err := k.Connect()
	// Make sure to close it later.
	defer k.Close()
	if res == 0 {
		log.Println("Состояние ККМ не извесно, выходим")
		return errors.New("Состояние ККМ не извесно")
	}
	errcode, _, err := k.SendCommand(0x13, k.AdminPassword[:])
	if err != nil {
		log.Printf("kkmBeep: %v", err)
		return err
	}
	if errcode > 0 {
		return errors.New(k.ErrState(errcode))
	}
	return nil
}

func kkmOpenShift(k *drv.KkmDrv) error {
	res, err := k.Connect()
	defer k.Close()
	if res == 0 {
		log.Println("Состояние ККМ не извесно, выходим")
		return err
	}
	errcode, _, err := k.SendCommand(0xe0, k.Password[:])
	if err != nil {
		log.Printf("kkmOpenShift: %v", err)
		return err
	}
	if errcode > 0 {
		return errors.New(k.ErrState(errcode))
	}
	return nil
}

func kkmGetStatus(k *drv.KkmDrv) error {
	res, err := k.Connect()
	defer k.Close()
	if res == 0 {
		log.Println("Состояние ККМ не извесно, выходим")
		return err
	}
	errcode, data, err := k.SendCommand(0x11, k.Password[:])
	if err != nil {
		log.Printf("kkmGetStatus: %v", err)
		return err
	}
	if errcode > 0 {
		return errors.New(k.ErrState(errcode))
	}
	/*
		data[0]=Порядковый номер оператора (1 байт) 1…30
		data[1:3] Версия ПО ККТ (2 байта)
		data[3:5] Сборка ПО ККТ (2 байта)
		data[5:8] Дата ПО ККТ (3 байта) ДД-ММ-ГГ
		data[8:9] Номер в зале (1 байт)
		data[9:11] Сквозной номер текущего документа (2 байта)
		data[11:13] Флаги ККТ (2 байта)
		data[13:14] Режим ККТ (1 байт)
		data[14:15] Подрежим ККТ (1 байт)
		data[15:16] Порт ККТ (1 байт)
		data[16:19] Дата (3 байта) ДД-ММ-ГГ
		data[19:22]Время (3 байта) ЧЧ-ММ-СС
		data[22:26] Заводской номер (4 байта) младшее длинное слово 6-байтного числа (см.ниже)
		data[26:28] Номер последней закрытой смены (2 байта)
		data[28:29] Количество перерегистраций (фискализаций) (1 байт)
		data[29:30] Количество оставшихся перерегистраций (фискализаций) (1 байт)
		data[30:36] ИНН (6 байт)
		data[36:38] Заводской номер (2 байта) старшее слово 6-байтного числа
	*/
	k.ParseState(data[13:14])

	return nil
}

/*
Передать произвольную TLV структуру привязанную к
операции
Код команды FF4DH . Длина сообщения: 6+N байт.
Пароль системного администратора: 4 байта
TLV Структура: N байт (мах 250 байт)
Ответ: FF4Dh Длина сообщения: 1 байт.
Код ошибки: 1 байт
Например, чтобы передать тэг 1008 «адрес покупателя» со значением 12345678 следует записать в
TLVData следующую последовательность байт: F0h 03h 08h 00h 31h 32h 33h 34h 35h 36h 37h 38h ,
где F0h 03h – код тэга, 08h 00h – длина сообщения
*/
func fnSendTLV(k *drv.KkmDrv, teg uint16, val []byte) error {
	res, err := k.Connect()
	defer k.Close()
	if res == 0 {
		log.Println("Состояние ККМ не извесно, выходим")
		return err
	}
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, teg)
	tlv := make([]byte, len(val)+6+2) //+2 len tlv
	copy(tlv, k.Password[:])
	copy(tlv[4:], b[:2]) //teg
	binary.LittleEndian.PutUint16(b, uint16(len(val)))
	copy(tlv[6:], b[:2]) //len
	copy(tlv[8:], val)
	errcode, _, err := k.SendCommand(0xff0c, tlv)
	if err != nil {
		log.Printf("kkmOpenShift: %v", err)
		return err
	}
	if errcode > 0 {
		return errors.New(k.ErrState(errcode))
	}
	return nil
}
