package main

import (
	"encoding/binary"
	"errors"
	"kkm-shtrih/drv"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/encoding/charmap"
)

//decodeWindows1251 из win1251 в uft-8
func decodeWindows1251(ba []uint8) []uint8 {
	dec := charmap.Windows1251.NewDecoder()
	out, _ := dec.Bytes(ba)
	return out
}

//encodeWindows1251 из uft-8 в win1251
func encodeWindows1251(ba string) []uint8 {
	enc := charmap.Windows1251.NewEncoder()
	out, _ := enc.String(ba)
	return []uint8(out)
}

// itob returns an 8-byte little endian representation of v.
func itob(v int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b
}

// btoi returns an 8-byte little endian representation of v.
func btoi(v []byte) int64 {
	b := binary.LittleEndian.Uint64(v)
	return int64(b)
}

//money2byte преобразует числа в слайс байт для передачи в ккм
func money2byte(money float64, digits int) []byte {
	//Разрядность денежных величин
	//Все суммы в данном разделе – целые величины, указанные в «мде». МДЕ –
	//минимальная денежная единица. С 01.01.1998 в Российской Федерации 1 МДЕ равна 1
	//копейке (до 01.01.1998 1 МДЕ была равна 1 рублю).
	//Формат передачи значений
	//Все числовые величины передаются в двоичном формате, если не указано другое.
	//Первым передается самый младший байт, последним самый старший байт.
	//При передаче даты (3 байта) сначала передаётся число (1 байт – ДД), затем месяц (2
	//байта – ММ), и последним – год (1 байт – ГГ).
	//При передаче времени (3 байта) первым байтом передаются часы (1 байт – ЧЧ), затем
	//минуты (1 байт – ММ), и последними передаются секунды (1 байт – СС).
	v := int64(math.Round(money * math.Pow10(digits))) //12.436*100=1243.6 = 1244 = int64(1244)
	return itob(v)
}

//q2byte преобразует количкство в слайс байт для передачи в ккм
func q2byte(q float64) []byte {
	//округляем до 3-х знаков
	v := int64(math.Round(q * 1000)) //12.43675*1000=12436.75 = 12437 = int64(12437)
	return itob(v)
}

//getIntParam возвращает параметр int
func getIntParam(c *gin.Context, param string, defaultval int) (int, error) {
	p, ok := c.GetQuery(param)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": param + " не указан"})
		return defaultval, errors.New(param + " не указан")
	}
	ret, err := strconv.Atoi(p)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": param + " должен быть числом"})
		return defaultval, errors.New(param + " должен быть числом")
	}
	return ret, nil
}

//getFloatParam возвращает параметр float64
func getFloatParam(c *gin.Context, param string, defaultval float64) (float64, error) {
	p, ok := c.GetQuery(param)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": param + " не указан"})
		return defaultval, errors.New(param + " не указан")
	}
	ret, err := strconv.ParseFloat(p, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": param + " должен быть числом"})
		return defaultval, errors.New(param + " должен быть числом")
	}
	return ret, nil
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
			descr = "Ошибка: " + k.ParseErrState(errcode)
		} else {
			num := append(data[:8], byte(0))
			ser := strconv.FormatUint(binary.LittleEndian.Uint64(num), 10)
			descr = "\nЗаводской номер: " + ser
			num = make([]byte, 8-len(data[8:]))
			copy(num, data[8:])
			rnm := strconv.FormatUint(binary.LittleEndian.Uint64(num), 10)
			descr = "\nPHM : " + rnm
			k.SetParam("-", "-", ser, rnm)
		}
	case "0x10":
		errcode, data, err = k.SendCommand(0x10, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ParseErrState(errcode)
		} else {
			ifl, fl := k.ParseFlag(binary.LittleEndian.Uint16(data[1:3]))
			imode, mode := k.ParseState(data[3])
			isub, submode := k.ParseSubState(data[4])
			k.SetState(data[3], data[4], binary.LittleEndian.Uint16(data[1:3]), 0)
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
			descr = "Ошибка: " + k.ParseErrState(errcode)
		} else {
			_, fl := k.ParseFlag(binary.LittleEndian.Uint16(data[11:13]))
			_, flfp := k.ParseFlagFP(data[29])
			_, mode := k.ParseState(data[13])
			_, submode := k.ParseSubState(data[14])
			k.SetState(data[13], data[14], binary.LittleEndian.Uint16(data[11:13]), data[29])
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
			descr = "Ошибка: " + k.ParseErrState(errcode)
		}
	case "0x15":
		if len(param) < 5 {
			param = append(param, byte(0))
		}
		errcode, data, err = k.SendCommand(0x15, param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ParseErrState(errcode)
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
			descr = "Ошибка: " + k.ParseErrState(errcode)
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
			descr = "Ошибка: " + k.ParseErrState(errcode)
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
			descr = "Ошибка: " + k.ParseErrState(errcode)
		} else {
			descr = "Ок"
		}
	case "0xfc", "0xFC", "getTipKKM":
		errcode, data, err = k.SendCommand(0xfc, []byte{})
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ParseErrState(errcode)
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
		errcode, err = k.ContinuePrint(param)
		if int(errcode) > 0 {
			descr = "Ошибка: " + k.ParseErrState(errcode)
		}
	}
	return
}

func kkmBeep(k *drv.KkmDrv) error {
	if k.ChkBusy(0) {
		err := errors.New("ККТ занята ")
		return err
	}
	//займем ккм
	procid := int(time.Now().Unix())
	k.SetBusy(procid)
	//освободим по завершению
	defer k.SetBusy(0)

	admpass := k.GetAdminPass()
	errcode, _, err := k.SendCommand(0x13, admpass)
	if err != nil {
		log.Printf("kkmBeep: %v", err)
		return err
	}
	if errcode > 0 {
		return errors.New(k.ParseErrState(errcode))
	}
	return nil
}

func kkmGetStatus(k *drv.KkmDrv, pid int) (int, error) {
	if k.ChkBusy(pid) {
		err := errors.New("ККТ занята ")
		return 1, err
	}
	admpass := k.GetAdminPass()
	errcode, data, err := k.SendCommand(0x10, admpass)
	if err != nil {
		return 1, err
	}
	if errcode > 0 {
		err = errors.New("Ошибка kkm: " + k.ParseErrState(errcode))
		return 1, err
	}
	k.SetState(data[3], data[4], binary.LittleEndian.Uint16(data[1:3]), data[8])
	//data[1:3] флаг
	//data[3:4] режим
	//data[4:5] подрежим
	//data[13] 	Причина завершения печати или промотки бумаги:
	//		0 – печать завершена успешно
	//		1 – произошел обрыв бумаги
	//		2 – ошибка принтера (перегрев головки, другая ошибка)
	//		5 – идет печать
	//op := make([]byte, 2)
	//op[0] = data[5]
	//op[1] = data[10]
	//Количество операций в чеке: " + strconv.FormatUint(uint64(binary.LittleEndian.Uint16(op)), 10)
	//Напряжение резервной батареи: " + strconv.FormatInt(int64(data[6]), 10)
	//Напряжение источника питания: " + strconv.FormatInt(int64(data[7]), 10)
	//Код ошибки ФП: " + strconv.FormatInt(int64(data[8]), 10)
	//Код ошибки ЭКЛЗ: " + strconv.FormatInt(int64(data[9]), 10)
	s, _ := k.ParseState(data[3])
	return s, nil
}

/*
Передать произвольную TLV структуру
Код команды FF0Ch (если тег относится к чеку)
либо
Передать произвольную TLV структуру привязанную к операции
Код команды FF4DH (если тег привязан к операции) .
Отличие в том, что в первом случае передаются теги привязанные к чеку
(такие как 1203 "ИНН кассира", 1226 "ИНН поставщика" и т.д),
а во втором - теги, которые привязаны именно к предмету расчета (т.е. к операции в чеке).
Это теги 1162, 1229 "Акциз", 1230 "код страны происхождения" и т.д.

Длина сообщения: 6+N байт.
Пароль системного администратора: 4 байта
TLV Структура: N байт (мах 250 байт)
Ответ: FF4Dh Длина сообщения: 1 байт.
Код ошибки: 1 байт
Например, чтобы передать тэг 1008 "адрес покупателя" со значением 12345678 следует записать в
TLVData следующую последовательность байт: F0h 03h 08h 00h 31h 32h 33h 34h 35h 36h 37h 38h ,
где F0h 03h – код тэга, 08h 00h – длина сообщения
.

*/
func setBusy(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	if kkm.GetState().Busy {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "device уже занят"})
		return
	}
	procid := int(time.Now().Unix())
	kkm.SetBusy(procid)
	hdata["procid"] = procid
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)
}

func release(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	sprocid, ok := c.GetQuery("procid")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "procid не указан"})
		return
	}
	procid, err := strconv.Atoi(sprocid)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "procid должен быть числом"})
		return
	}
	if kkm.GetState().ProcID != procid {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "procid не верен"})
		return
	}
	state, err := kkm.GetStatus()
	if err != nil {
		if state == 88 {
			//ждем ппродолжить печать
			kkm.ContinuePrint([]byte{})
		}
		kkm.SetBusy(0)
	}

	//if state>=80 {
	//аннулировать чек и закрыть
	//}
	kkm.SetBusy(0)
	hdata["procid"] = 0
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)
}

func openCheck(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	sprocid, ok := c.GetQuery("procid")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "procid не указан"})
		return
	}
	procid, err := strconv.Atoi(sprocid)
	if kkm.ChkBusy(procid) {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}

	var pass []byte
	spass, ok := c.GetQuery("pass")
	if !ok {
		pass = kkm.GetAdminPass()
	}
	ipass, err := strconv.Atoi(spass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Пароль должен быть числовым"})
		return
	}
	pass = itob(int64(ipass))[:4]
	errcode, err := kkm.CancelCheck(pass)
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}

	errcode, err = kkm.FNGetStatus()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	if errcode == 0 {
		fnstate := kkm.FNGetFNState()
		ShiftState := int(fnstate.FNSessionState + 1) //1 - Закрыта 2 - Открыта 3 - Истекла
		if ShiftState != 2 {
			if ShiftState == 3 {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": "Смена истекла, необходимо закрытие"})
			} else {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": "Смена закрыта"})
			}
			return
		}
	} else {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}

	checkType, ok := c.GetQuery("CheckType")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "CheckType не указан"})
		return
	}
	var chktype byte = 0
	//Услуга (1-товар, 2-акцизный товар; 3 - работа; 4-услуга....)
	switch checkType {
	case "0": //продажа
		chktype = 0
	case "2": //возврат продажи
		chktype = 2
	case "1": //покупка
		chktype = 1
	case "3": //возврат покупки
		chktype = 3
	default:
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "CheckType не верен"})
		return
	}
	errcode, err = kkm.OpenCheck(pass, chktype)
	if errcode > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}

	hdata["procid"] = procid
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)

}

func fnOperation(c *gin.Context) {
	/*
			Если Summ1Enabled имеет значение "ложь", то сумма рассчитывается кассой как цена * количество, в противном случае сумма операции берётся из значений Summ1 и не должна отличаться более чем на + -1 коп от рассчитанной кассой.
		В режиме начисления налогов 1 (1 Таблица) налоги на позицию на чек должны передаваться из верхнего ПО. Если TaxValueEnabled имеет значение "Ложь", то считается, что сумма налога на позицию не указана, в противном случае сумма налога учитывается ФР и передаётся в ОФД. Для налогов 3 и 4 сумма налога всегда считается равной нулю и в ОФД не передаётся.
		Если строка начинается символами //, то она передаётся на сервер ОФД, но не печатается на кассе.
		Количество округляется до 6 знаков после запятой. Используемые свойства:
		Пароль - пароль оператора,
		CheckType - тип операции,
		Quantity - количество (до 6 знаков после запятой),
		Price - цена (в копейках),
		Summ1 - сумма операции (в копейках),
		Summ1Enabled - использовать сумму операции,
		TaxValue - сумма нолога (в копейках),
		TaxValueEnabled - использовать сумму налога,
		Tax1 - налоговая ставка,
		Department - отдел (0..16 режим свободной продажи, 255 - режим продажи по коду товара),
		PaymentTypeSign - признак метода расчета,
		PaymentItemSign - признак предмета расчета,
		StringForPrinting - наименование товара.
	*/
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	procid, err := getIntParam(c, "procid", 0)
	if err != nil {
		return
	}
	if kkm.ChkBusy(procid) {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}
	var pass []byte
	spass, ok := c.GetQuery("pass")
	if !ok {
		pass = kkm.GetAdminPass()
	}
	ipass, err := strconv.Atoi(spass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Пароль должен быть числовым"})
		return
	}
	pass = itob(int64(ipass))[:4]
	checkType, err := getIntParam(c, "CheckType", 0)
	if err != nil {
		return
	}
	quantity, err := getFloatParam(c, "Quantity", 0.0)
	if err != nil {
		return
	}
	price, err := getFloatParam(c, "Price", 0.0)
	if err != nil {
		return
	}
	summ1, err := getFloatParam(c, "Summ1", 0.0)
	if err != nil {
		return
	}
	taxval, err := getFloatParam(c, "TaxValue", 0.0)
	if err != nil {
		return
	}
	tax1 := c.DefaultQuery("Tax1", "4")
	//Department - отдел (0..16 режим свободной продажи, 255 – режим продажи по коду товара),
	department, err := getIntParam(c, "Department", 0)

	paymentTypeSign, err := getIntParam(c, "PaymentTypeSign", 4) //Полный расчет
	paymentItemSign, err := getIntParam(c, "PaymentItemSign", 1) //товар
	stringForPrinting := c.DefaultQuery("StringForPrinting", "")
	/*PaymentTypeSign - признак способа расчета,
	1	Предоплата 100%
	2	Частичная предоплата
	3	Аванс
	4	Полный расчет
	5	Частичный расчет
	6	Передача в кредит
	7	Оплата кредита*/
	//PaymentItemSign - признак предмета расчета,
	//StringForPrinting - наименование товара.
	errcode, _ := kkm.FNOperation(pass, checkType, quantity, price, summ1, taxval, tax1, department, paymentTypeSign, paymentItemSign, stringForPrinting)
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	hdata["procid"] = procid
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)
}

func printString(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	procid, err := getIntParam(c, "procid", 0)
	if err != nil {
		return
	}
	if kkm.ChkBusy(procid) {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}
	var pass []byte
	spass, ok := c.GetQuery("pass")
	if !ok {
		pass = kkm.GetAdminPass()
	}
	ipass, err := strconv.Atoi(spass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Пароль должен быть числовым"})
		return
	}
	pass = itob(int64(ipass))[:4]
	errcode, err := kkm.CancelCheck(pass)
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	hdata["procid"] = procid
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)
}

func cancelCheck(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	procid, err := getIntParam(c, "procid", 0)
	if err != nil {
		return
	}
	if kkm.ChkBusy(procid) {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}
	var pass []byte
	spass, ok := c.GetQuery("pass")
	if !ok {
		pass = kkm.GetAdminPass()
	}
	ipass, err := strconv.Atoi(spass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Пароль должен быть числовым"})
		return
	}
	pass = itob(int64(ipass))[:4]
	errcode, err := kkm.CancelCheck(pass)
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	hdata["procid"] = procid
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)
}

func closeCheck(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	procid, err := getIntParam(c, "procid", 0)
	if err != nil {
		return
	}
	if kkm.ChkBusy(procid) {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}
	var pass []byte
	spass, ok := c.GetQuery("pass")
	if !ok {
		pass = kkm.GetAdminPass()
	}
	ipass, err := strconv.Atoi(spass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Пароль должен быть числовым"})
		return
	}
	pass = itob(int64(ipass))[:4]
	errcode, err := kkm.CancelCheck(pass)
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	taxsystem, err := getIntParam(c, "taxsystem", 0)
	if err != nil {
		return
	}
	//taxsystem = Код системы налогообложения.
	//0	Общая
	//1	Упрощенная (Доход)
	//2	Упрощенная (Доход минус Расход)
	//3	Единый налог на вмененный доход
	//4	Единый сельскохозяйственный налог
	//5	Патентная система налогообложения
	//summa1,summa2...summa16    tax1,tax2
	summa := make(map[int]float64)
	vta := make(map[string]float64)
	for i := int64(1); i <= 16; i++ {
		p := "summ" + strconv.FormatInt(i, 10)
		v := c.Query(p)
		if len(v) > 0 {
			f, err := strconv.ParseFloat(v, 64)
			if err == nil {
				summa[int(i)] = f
			}
		}
	}
	//Налог 1=НДС 18%,	Налог 2 =НДС 10%,налог 3 =НДС 0%,налог 4 =(Без НДС),Налог 5 = 18/118,	Налог 6 = (НДС расч. 10/110)
	for i := int64(1); i <= 6; i++ {
		t := strconv.FormatInt(i, 10)
		p := "taxvalue" + t
		v := c.Query(p)
		if len(v) > 0 {
			f, err := strconv.ParseFloat(v, 64)
			if err == nil {
				vta[t] = f
			}
		}
	}
	printstring := c.Query("printstring")

	retsum, checkNumber, fiscalSign, dtime, errcode, err := kkm.CloseCheck(pass, summa, vta, byte(taxsystem), 0, printstring)
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	hdata["retsum"] = retsum
	hdata["checkNumber"] = checkNumber
	hdata["fiscalSign"] = fiscalSign
	hdata["datetime"] = dtime

	hdata["procid"] = procid
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)
}

func fnSendTagOperation(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	procid, err := getIntParam(c, "procid", 0)
	if err != nil {
		return
	}
	if kkm.ChkBusy(procid) {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}
	var pass []byte
	spass, ok := c.GetQuery("pass")
	if !ok {
		pass = kkm.GetAdminPass()
	}
	ipass, err := strconv.Atoi(spass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Пароль должен быть числовым"})
		return
	}
	pass = itob(int64(ipass))[:4]
	errcode, err := kkm.CancelCheck(pass)
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	steg, ok := c.GetQuery("teg")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Тег должен быть числовым"})
	}
	teg, err := strconv.Atoi(steg)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Тег должен быть числовым"})
		return
	}
	val, ok := c.GetQuery("val")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Не указано значение тега"})
	}

	errcode, err = kkm.FNSendTLVOperation(pass, uint16(teg), encodeWindows1251(val))
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	hdata["procid"] = procid
	hdata["teg"] = teg
	hdata["val"] = val
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)
}

func fnSendTag(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	sprocid, ok := c.GetQuery("procid")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "procid не указан"})
		return
	}
	procid, err := strconv.Atoi(sprocid)
	if kkm.ChkBusy(procid) {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}
	var pass []byte
	spass, ok := c.GetQuery("pass")
	if !ok {
		pass = kkm.GetAdminPass()
	}
	ipass, err := strconv.Atoi(spass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Пароль должен быть числовым"})
		return
	}
	pass = itob(int64(ipass))[:4]
	errcode, err := kkm.CancelCheck(pass)
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	steg, ok := c.GetQuery("teg")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Тег должен быть числовым"})
	}
	teg, err := strconv.Atoi(steg)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Тег должен быть числовым"})
		return
	}
	val, ok := c.GetQuery("val")
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "Не указано значение тега"})
	}

	errcode, err = kkm.FNSendTLV(pass, uint16(teg), encodeWindows1251(val))
	if errcode > 0 {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		return
	}
	hdata["procid"] = procid
	hdata["teg"] = teg
	hdata["val"] = val
	hdata["error"] = false
	hdata["message"] = "ok"
	c.JSON(http.StatusOK, hdata)
}
