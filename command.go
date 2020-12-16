package main

import (
	"encoding/binary"
	"errors"
	"kkm-shtrih/drv"
	"log"
	"strconv"

	"golang.org/x/text/encoding/charmap"
)

//decodeWindows1251 из win1251 в uft-8
func decodeWindows1251(ba []uint8) []uint8 {
	dec := charmap.Windows1251.NewDecoder()
	out, _ := dec.Bytes(ba)
	return out
}

//encodeWindows1251 из uft-8 в win1251
func encodeWindows1251(ba []uint8) []uint8 {
	enc := charmap.Windows1251.NewEncoder()
	out, _ := enc.String(string(ba))
	return []uint8(out)
}

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

func kkmRunFunction(k *drv.KkmDrv, fname string, param []byte) (errcode byte, data []byte, descr string, err error) {
	if k.Busy == true {
		errcode = 0
		err = errors.New("ККТ занята ")
		return
	}
	switch fname {
	//case "0x01":
	//	errcode, data, err = k.SendCommand(0x01, param)
	//case "0x02":
	//	errcode, data, err = k.SendCommand(0x02, param)
	case "0x03":
		errcode, data, err = k.SendCommand(0x03, param)
	case "0x0f", "0x0F":
		//Запрос длинного заводского номера и длинного РНМ
		//Заводской номер (7 байт) 00000000000000…999999999999991
		//РНМ (7 байт) 00000000000000…99999999999999
		errcode, data, err = k.SendCommand(0x0f, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		} else {
			num := append(data[:8], byte(0))
			descr = "\nЗаводской номер: " + strconv.FormatUint(binary.LittleEndian.Uint64(num), 10)
			num = make([]byte, 8-len(data[8:]))
			copy(num, data[8:])
			descr = "\nPHM : " + strconv.FormatUint(binary.LittleEndian.Uint64(num), 10)
		}
	case "0x10":
		errcode, data, err = k.SendCommand(0x10, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		} else {
			ifl, fl := k.ParseFlag(data[1:3])
			imode, mode := k.ParseState(data[3:4])
			isub, submode := k.ParseSubState(data[4:5])
			/*
				Причина завершения печати или промотки бумаги:
				0 – печать завершена успешно
				1 – произошел обрыв бумаги
				2 – ошибка принтера (перегрев головки, другая ошибка)
				5 – идет печать
			*/
			prres := ""
			switch int(data[13]) {
			case 0:
				prres = "печать завершена успешно"
			case 1:
				prres = "произошел обрыв бумаги"
			case 3:
				prres = "пошибка принтера (перегрев головки, другая ошибка)"
			case 5:
				prres = "идет печать"
			}
			descr = "----------------------------------------\nКраткий запрос состояния:\n----------------------------------------"
			descr = descr + "\nРежим ККТ: " + strconv.FormatInt(int64(imode), 10) + ", " + mode + "\nПодрежим: " + strconv.FormatInt(int64(isub), 10) + ", " + submode
			op := make([]byte, 2)
			op[0] = data[5]
			op[1] = data[10]
			descr = descr + "\n----------------------------------------\nКоличество операций в чеке: " + strconv.FormatUint(uint64(binary.LittleEndian.Uint16(op)), 10)
			descr = descr + "\nНапряжение резервной батареи: " + strconv.FormatInt(int64(data[6]), 10)
			descr = descr + "\nНапряжение источника питания: " + strconv.FormatInt(int64(data[7]), 10)
			descr = descr + "\nКод ошибки ФП: " + strconv.FormatInt(int64(data[8]), 10)
			descr = descr + "\nКод ошибки ЭКЛЗ: " + strconv.FormatInt(int64(data[9]), 10)
			//Зарезервировано (3 байта)
			descr = descr + "\nРезультат последней печати: " + prres
			descr = descr + "\n----------------------------------------\nФлаги: " + strconv.FormatInt(int64(ifl), 10) + ", " + fl
		}
	case "0x11":
		errcode, data, err = k.SendCommand(0x11, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		} else {
			_, fl := k.ParseFlag(data[11:13])
			_, flfp := k.ParseFlagFP(data[29:30])
			_, mode := k.ParseState(data[13:14])
			_, submode := k.ParseSubState(data[14:15])
			descr = "----------------------------------------\nЗапрос состояния:\n----------------------------------------\n"
			descr = descr + "Версия ПО ККТ: " + string(data[1]) + "." + string(data[2])
			descr = descr + "\nСборка ПО ККТ: " + strconv.FormatUint(uint64(binary.LittleEndian.Uint16(data[3:5])), 10)
			descr = descr + "\nДата ПО ККТ: " + strconv.FormatUint(uint64(data[5]), 10) + "." + strconv.FormatUint(uint64(data[6]), 10) + "." + strconv.FormatUint(uint64(data[7]), 10)
			descr = descr + "\nНомер в зале: " + strconv.FormatUint(uint64(data[8]), 10)
			descr = descr + "\nСквозной номер текущего документа: " + strconv.FormatUint(uint64(binary.LittleEndian.Uint16(data[9:11])), 10)
			descr = descr + "\n----------------------------------------"
			descr = descr + "\nРежим ККТ: " + mode + "\nПодрежим: " + submode + "\nФлаги: " + fl
			descr = descr + "\n----------------------------------------"
			descr = descr + "\nПорт ККТ: " + strconv.FormatUint(uint64(data[15]), 10)
			descr = descr + "\nВерсия ПО ФП: " + string(data[16]) + "." + string(data[17])
			descr = descr + "\nСборка ПО ФП: " + strconv.FormatUint(uint64(binary.LittleEndian.Uint16(data[18:20])), 10)
			descr = descr + "\nДата ПО ФП: " + strconv.FormatUint(uint64(data[20]), 10) + "." + strconv.FormatUint(uint64(data[21]), 10) + "." + strconv.FormatUint(uint64(data[22]), 10)
			descr = descr + "\nДата: " + strconv.FormatUint(uint64(data[23]), 10) + "." + strconv.FormatUint(uint64(data[24]), 10) + "." + strconv.FormatUint(uint64(data[25]), 10)
			descr = descr + "\nВремя: " + strconv.FormatUint(uint64(data[26]), 10) + "." + strconv.FormatUint(uint64(data[27]), 10) + "." + strconv.FormatUint(uint64(data[28]), 10)
			descr = descr + "\nЗаводской номер: " + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(data[30:34])), 10)
			descr = descr + "\nНомер последней закрытой смены: " + strconv.FormatUint(uint64(binary.LittleEndian.Uint16(data[34:36])), 10)
			descr = descr + "\nСвободных записей в ФП : " + strconv.FormatUint(uint64(binary.LittleEndian.Uint16(data[36:38])), 10)
			descr = descr + "\nКоличество перерегистраций (фискализаций) : " + strconv.FormatUint(uint64(data[38]), 10)
			descr = descr + "\nКоличество оставшихся перерегистраций : " + strconv.FormatUint(uint64(data[39]), 10)
			descr = descr + "\n----------------------------------------"
			descr = descr + "\nФлаги ФП : " + flfp
			inn := make([]byte, 8)
			copy(inn, data[40:46])
			descr = descr + "\nИНН: " + strconv.FormatUint(uint64(binary.LittleEndian.Uint64(inn)), 10)
		}
	case "0x13", "beep":
		errcode, data, err = k.SendCommand(0x13, param)
		descr = "ok!"
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		}
	case "0x15":
		if len(param) < 5 {
			param = append(param, byte(0))
		}
		errcode, data, err = k.SendCommand(0x15, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		} else {
			speed := []string{"2400", "4800", "9600", "19200", "38400", "57600", "115200", "2304001", "4608001", "9216001"}
			descr = "Скорость обмена: " + speed[uint64(data[0])]
			to := ""
			switch {
			case uint64(data[1]) <= 150:
				to = strconv.FormatUint(uint64(data[1]), 10)
			case uint64(data[1]) > 150 && uint64(data[1]) < 250:
				to = strconv.FormatUint(150*(uint64(data[1])-150), 10)
			case uint64(data[1]) >= 250 && uint64(data[1]) <= 255:
				to = strconv.FormatUint(15000*(uint64(data[1])-150), 10)
			}
			descr = descr + "\nТайм аут приема байта: " + to + " мс"
		}
	case "0x19":
		//тестовый прогон, время прогона в минутах
		if len(param) < 5 {
			param = append(param, byte(1))
		}
		errcode, data, err = k.SendCommand(0x19, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		} else {
			descr = "Ок"
		}
	case "0x25":
		//Отрезка чека, тип отрезки (1 байт) «0» – полная, «1» – неполная
		if len(param) < 5 {
			param = append(param, byte(0))
		}
		errcode, data, err = k.SendCommand(0x25, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		} else {
			descr = "Ок"
		}
	case "0x28":
		//открыть денежный ящик, номер ящика 0,1
		if len(param) < 5 {
			param = append(param, byte(0))
		}
		errcode, data, err = k.SendCommand(0x28, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		} else {
			descr = "Ок"
		}
	case "0xfc", "0xFC", "getTipKKM":
		errcode, data, err = k.SendCommand(0xfc, []byte{})
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		} else {
			descr = "Тип устройства: " + strconv.FormatUint(uint64(data[0]), 10)
			descr = descr + "\nПодтип устройства: " + strconv.FormatUint(uint64(data[1]), 10)
			descr = descr + "\nВерсия протокола: " + strconv.FormatUint(uint64(data[2]), 10)
			descr = descr + "\nПодверсия протокола: " + strconv.FormatUint(uint64(data[3]), 10)
			descr = descr + "\nМодель устройства: " + strconv.FormatUint(uint64(data[4]), 10)
			lang := []string{"русский", "английский", "эстонский", "казахский", "белорусский", "армянский", "грузинский", "украинский", "киргизский", "туркменский", "молдавский", "другой", "ино", "вооще не понятно"}
			descr = descr + "\nЯзык устройства: " + lang[uint64(data[5])]
			//Название устройства – строка символов в кодировке WIN1251.
			descr = descr + "\nИмя устройства: " + string(decodeWindows1251(data[6:]))

		}
	case "0xB0", "0xb0":
		//Продолжение печати
		errcode, data, err = k.SendCommand(0x80, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ErrState(errcode)
		}
	}
	k.Busy = false
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
		log.Printf("kkmOpenShift: %v", err)
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
Например, чтобы передать тэг 1008 "адрес покупателя" со значением 12345678 следует записать в
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
