package main

import (
	"time"

	"net/http"

	"github.com/gin-gonic/gin"
)

//OpenCashDrawer открыть денежный ящик номер ящика 0,1
func openCashDrawer(c *gin.Context) {
	deviceID := c.Param("DeviceID")

	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}

	if kkm.ChkBusy(0) {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		return
	}
	//займем ккм
	procid := int(time.Now().Unix())
	kkm.SetBusy(procid)
	//освободим по завершению
	defer kkm.SetBusy(0)
	admpass := kkm.GetAdminPass()
	param := make([]byte, 5)
	copy(param, admpass[:4])
	param[5] = byte(0)
	errcode, _, _ := kkm.SendCommand(0x28, param)
	if errcode > 0 {
		c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
	} else {
		c.XML(http.StatusOK, gin.H{"error": false, "message": "ok"})
	}
}

func getLineLength(c *gin.Context) {
	/*Прочитать параметры шрифта
	  Команда: 26H. Длина сообщения: 6 байт.
	  Пароль системного администратора (4 байта)
	  Номер шрифта (1 байт)
	  Ответ: 26H. Длина сообщения: 7 байт.
	  Код ошибки (1 байт)
	  Ширина области печати в точках (2 байта)
	  Ширина символа с учетом межсимвольного интервала в точках (1 байт)
	  Высота символа с учетом межстрочного интервала в точках (1 байт)
	  Количество шрифтов в ККТ (1 байт)*/
	deviceID := c.Param("DeviceID")

	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}

	json, ok := c.GetQuery("format")
	if !ok {
		json = "xml"
	}
	procid, err := getIntParam(c, "procid", 0)
	/*
		if err != nil {
			if json == "json" {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
			} else {
				c.XML(http.StatusOK, gin.H{"error": true, "message": err.Error()})
			}
			return
		}
	*/
	if kkm.ChkBusy(procid) {
		if json == "json" {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		} else {
			c.XML(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		}
		return
	}

	//займем ккм
	if procid == 0 {
		procid = int(time.Now().Unix())
		//освободим по завершению
		defer kkm.SetBusy(0)
	}
	kkm.SetBusy(procid)

	admpass := kkm.GetAdminPass()

	param := make([]byte, 5)
	copy(param, admpass[:4])
	param[5] = byte(1) //номер шрифта
	errcode, data, _ := kkm.SendCommand(0x26, param)
	if errcode > 0 {
		if json == "xml" {
			c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		} else {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		}
		return
	}

	wlinepoint := btoi(data[:2])
	wfont := btoi(data[2:3])
	linew := int(wlinepoint / wfont)
	if json == "xml" {
		c.XML(http.StatusOK, gin.H{"error": false, "message": "ok", "LineLength": linew})
	} else {
		c.JSON(http.StatusOK, gin.H{"error": false, "message": "ok", "LineLength": linew})
	}

}

//cashInOutcome печать чека внесения/выемки
func cashInOutcome(c *gin.Context) {
	deviceID := c.Param("DeviceID")

	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	json, ok := c.GetQuery("format")
	if !ok {
		json = "xml"
	}
	procid, err := getIntParam(c, "procid", 0)
	/*
		if err != nil {
			if json == "json" {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
			} else {
				c.XML(http.StatusOK, gin.H{"error": true, "message": err.Error()})
			}
			return
		}
	*/
	if kkm.ChkBusy(procid) {
		if json == "json" {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		} else {
			c.XML(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		}
		return
	}

	//займем ккм
	if procid == 0 {
		procid = int(time.Now().Unix())
		//освободим по завершению
		defer kkm.SetBusy(0)
	}
	kkm.SetBusy(procid)

	admpass := kkm.GetAdminPass()
	/*
	   <?xml version="1.0" encoding="UTF-8"?>
	    <InputParameters>
	   	<Parameters CashierName="Иванов И.П." CashierINN="32456234523452"/>
	    </InputParameters>
	   Внесение
	   Команда: 50H. Длина сообщения: 10 байт.
	   Пароль оператора (4 байта)
	   Сумма (5 байт)
	   Ответ: 50H. Длина сообщения: 5 байт.
	   Код ошибки (1 байт)
	   Порядковый номер оператора (1 байт) 1…30
	   Сквозной номер документа (2 байта)
	   Выплата
	   Команда: 51H. Длина сообщения: 10 байт.
	   Пароль оператора (4 байта)
	   Сумма (5 байт)
	   Ответ: 51H. Длина сообщения: 5 байт.
	   Код ошибки (1 байт)
	   Порядковый номер оператора (1 байт) 1…30
	   Сквозной номер документа (2 байта)
	*/
	amount, err := getFloatParam(c, "Amount", 0.0)
	if err != nil {
		if json == "json" {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": "Сумма внесения/выемки не должна быть нулевой"})
		} else {
			c.XML(http.StatusOK, gin.H{"error": true, "message": "умма внесения/выемки не должна быть нулевой"})
		}
		return
	}
	param := make([]byte, 10)
	cmd := uint16(0x50)
	if amount < 0 {
		cmd = 0x51
		amount = -amount
	}
	copy(param, admpass[:4])
	copy(param[4:], money2byte(amount, DIGITS))
	errcode, data, err := kkm.SendCommand(cmd, param)
	if errcode > 0 {
		if json == "xml" {
			c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		} else {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		}
		return
	}
	//data[0] - oper pass

	if json == "xml" {
		c.XML(http.StatusOK, gin.H{"error": false, "message": "ok", "docnum": btoi(data[1:])})
	} else {
		c.JSON(http.StatusOK, gin.H{"error": false, "message": "ok", "docnum": btoi(data[1:])})
	}
}

//printCheckCopy Запрос квитанции о получении данных в ОФД по номеру
func printCheckCopy(c *gin.Context) {
	deviceID := c.Param("DeviceID")

	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}

	json, ok := c.GetQuery("format")
	if !ok {
		json = "xml"
	}
	procid, err := getIntParam(c, "procid", 0)
	/*if err != nil {
		if json == "json" {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		} else {
			c.XML(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		}
		return
	}*/
	if kkm.ChkBusy(procid) {
		if json == "json" {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		} else {
			c.XML(http.StatusOK, gin.H{"error": true, "message": "ККТ занята"})
		}
		return
	}

	//займем ккм
	if procid == 0 {
		procid = int(time.Now().Unix())
		//освободим по завершению
		defer kkm.SetBusy(0)
	}
	kkm.SetBusy(procid)

	admpass := kkm.GetAdminPass()

	checkNumber, err := getIntParam(c, "CheckNumber", 0)
	if err != nil {
		if json == "json" {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		} else {
			c.XML(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		}
		return
	}
	param := make([]byte, 8)

	copy(param, admpass[:4])
	copy(param[4:], itob(int64(checkNumber)))
	/* Код команды FF3Сh . Длина сообщения: 11 байт.
	Пароль системного администратора: 4 байта [0:4]
	Номер фискального документа: 4 байта	[4:8]
	Ответ: FF3Сh Длина сообщения: 1+N байт.
	Код ошибки: 1 байт
	Квитанция: N байт
	*/
	errcode, data, err := kkm.SendCommand(0xff3c, param)
	if errcode > 0 {
		if json == "xml" {
			c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		} else {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		}
		return
	}
	//data[0] - oper pass

	if json == "xml" {
		c.XML(http.StatusOK, gin.H{"error": false, "message": "ok", "docval": data[:]})
	} else {
		c.JSON(http.StatusOK, gin.H{"error": false, "message": "ok", "docval": data[:]})
	}
}
