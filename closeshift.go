package main

import (
	"encoding/xml"
	"log"
	"time"

	"net/http"

	"github.com/gin-gonic/gin"
)

//CloseShift закрыть смену
func CloseShift(c *gin.Context) {
	/*<?xml version="1.0" encoding="UTF-8"?>
	 <InputParameters>
		<Parameters CashierName="Иванов И.П." CashierINN="32456234523452"/>
	 </InputParameters>*/
	type Parameters struct {
		CashierName  string `xml:"CashierName,attr" binding:"required"` //ФИО и должность уполномоченного лица для проведения операции
		CashierINN   string `xml:"CashierINN,attr" binding:"-"`         //ИНН уполномоченного лица для проведения операции
		SaleAddress  string `xml:"SaleAddress,attr" binding:"-"`        //Адрес проведения расчетов
		SaleLocation string `xml:"SaleLocation,attr" binding:"-"`       //Место проведения расчетов
	}
	type InputParameters struct {
		XMLName    xml.Name `xml:"InputParameters"`
		Parameters `xml:"Parameters"`
	}
	type OperationCounters struct {
		//Количество чеков по операции данного типа
		CheckCount int `xml:"CheckCount" binding:"required"`
		//Итоговая сумма чеков по операциям данного типа
		TotalChecksAmount float64 `xml:"TotalChecksAmount" binding:"required"`
		//Количество чеков коррекции по операции данного типа
		CorrectionCheckCount int `xml:"CorrectionCheckCount" binding:"required"`
		//Итоговая сумма чеков коррекции по операциям данного типа
		TotalCorrectionChecksAmount float64 `xml:"TotalCorrectionChecksAmount" binding:"required"`
	}
	type OutParameters struct {
		ShiftNumber             int    `xml:"ShiftNumber,attr" binding:"required"`      //Номер открытой смены/Номер закрытой смены
		CheckNumber             int    `xml:"CheckNumber,attr" binding:"-"`             //Номер последнего фискального документа
		ShiftClosingCheckNumber int    `xml:"ShiftClosingCheckNumber,attr" binding:"-"` //Номер последнего чека за смену
		DateTime                string `xml:"DateTime,attr" binding:"required"`         //Дата и время формирования фискального документа
		//Состояние смены
		//1 - Закрыта
		//2 - Открыта
		//3 - Истекла
		ShiftState int `xml:"ShiftState,attr" binding:"required"`
		//Счетчики операций по типу "приход"
		//(код 1, Таблица 25 документа ФФД)
		CountersOperationType1 OperationCounters `xml:"CountersOperationType1" binding:"-"`
		//Счетчики операций по типу "возврат прихода"
		//(код 2, Таблица 25 документа ФФД)
		CountersOperationType2 OperationCounters `xml:"CountersOperationType2" binding:"-"`
		//Счетчики операций по типу "расход"
		//(код 3, Таблица 25 документа ФФД)
		CountersOperationType3 OperationCounters `xml:"CountersOperationType3" binding:"-"`
		//Счетчики операций по типу "возврат расхода"
		//(код 4, Таблица 25 документа ФФД)
		CountersOperationType4 OperationCounters `xml:"CountersOperationType4" binding:"-"`
		//Остаток наличных денежных средств в кассе
		CashBalance float64 `xml:"CashBalance,attr" binding:"-"`
		//Количество непереданных документов
		BacklogDocumentsCounter int `xml:"BacklogDocumentsCounter,attr" binding:"-"`
		//Номер первого непереданного документа
		BacklogDocumentFirstNumber int `xml:"BacklogDocumentFirstNumber,attr" binding:"-"`
		//Дата и время первого из непереданных документов
		BacklogDocumentFirstDateTime string `xml:"BacklogDocumentFirstDateTime,attr" binding:"-"`
		//Признак необходимости срочной замены ФН
		FNError    bool `xml:"FNError,attr" binding:"required"`
		FNOverflow bool `xml:"FNOverflow,attr" binding:"required"` //Признак переполнения памяти ФН
		FNFail     bool `xml:"FNFail,attr" binding:"required"`     //Признак исчерпания ресурса ФН
	}
	type OutputParameters struct {
		XMLName       xml.Name `xml:"OutputParameters"`
		OutParameters `xml:"Parameters"`
	}

	var inp = InputParameters{}
	var out = OutputParameters{}

	deviceID := c.Param("DeviceID")
	out.FNError = false    //Признак необходимости срочной замены ФН
	out.FNOverflow = false //Признак переполнения памяти ФН
	out.FNFail = false     //Признак исчерпания ресурса ФН

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
	if err = c.ShouldBindXML(&inp); err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	istate, err := kkm.GetStatus()
	if err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	switch istate {
	case 2:
		out.ShiftState = 2
	case 3:
		out.ShiftState = 3
	case 4:
		out.ShiftState = 1
	}

	errcode, err := kkm.FNGetStatus()
	if err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errcode == 0 {
		fnstate := kkm.FNGetFNState()
		out.ShiftState = int(fnstate.FNSessionState + 1) //1 - Закрыта 2 - Открыта 3 - Истекла
		switch fnstate.FNWarningFlags {
		case 1:
			out.FNError = true
		case 2:
			out.FNFail = true
		case 4:
			out.FNOverflow = true
		}

	}

	if out.ShiftState == 1 {
		//смена уже закрыта, заполним выходные параметры и выходим
		c.XML(http.StatusBadRequest, gin.H{"error": "смена уже закрыта"})
		return
	}

	/*
		Начать закрытие смены
		Код команды FF42h . Длина сообщения: 6 байт.
		Пароль системного администратора: 4 байта
		Ответ: FF42h Длина сообщения: 1 байт.
		Код ошибки: 1 байт
		Закрыть смену в ФН
		Код команды FF43h . Длина сообщения: 6 байт.
		Пароль системного администратора: 4 байт
		Ответ: FF43h Длина сообщения: 11 (16) байт1
	*/
	errcode, _, err = kkm.SendCommand(0xff42, admpass)
	if err != nil {
		log.Printf("kkmCloseShift: %v", err)
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errcode > 0 {
		//старая ккм, просто close смену
		errcode, _, err := kkm.SendCommand(0x41, admpass)
		if err != nil {
			log.Printf("kkmCloseShift: %v", err)
			c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errcode > 0 {
			c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
			return
		}
		//запросим параметры смены
		state := kkm.GetState()
		out.ShiftNumber = int(state.LastSession)
		out.ShiftState = 1
		kkm.SetState(1, state.SubState, state.Flag, state.FlagFP)
	} else {
		//отправим tlv с параметрами и close смену ФН
		//тег 1203 ИНН Кассира
		//Тег 1021 — кассир. В печатных документах — «КАССИР». Сюда должны вноситься «должность и фамилия лица, осуществившего расчет с покупателем
		if len(inp.CashierINN) > 0 {
			kkm.FNSendTLV(admpass, 1203, []byte(inp.CashierINN))
		}
		if len(inp.CashierName) > 0 {
			kkm.FNSendTLV(admpass, 021, []byte(encodeWindows1251(inp.CashierName)))
		}
		if len(inp.SaleAddress) > 0 {
			kkm.FNSendTLV(admpass, 1009, []byte(encodeWindows1251(inp.SaleAddress)))
		}
		if len(inp.SaleLocation) > 0 {
			kkm.FNSendTLV(admpass, 1187, []byte(encodeWindows1251(inp.SaleLocation)))
		}
		//теперь close
		/*Код ошибки: 1 байт
		Номер только что закрытой смены: 2 байта
		Номер ФД :4 байта
		Фискальный признак: 4 байта
		Дата и время: 5 байт DATE_TIME может отсутстыовать*/
		errcode, data, err := kkm.SendCommand(0xff43, admpass)
		if err != nil {
			log.Printf("kkmCloseShift: %v", err)
			c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errcode > 0 {
			if errcode == 0x05 { // "Закончен срок эксплуатации ФН",
				out.FNError = true
			}
			if errcode == 0x06 { //Архив ФН переполнен
				out.FNOverflow = true
			}
			if errcode == 0x12 { // ФН Исчерпан ресурс КС(криптографического сопроцессора) Требуется закрытие фискального режима
				out.FNError = true
				out.FNFail = true
			}
			//c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
			//return
		}
		//Номер новой открытой смены: 2 байта
		out.ShiftNumber = int(btoi(data[:2]))
		//Номер ФД :4 байта
		out.CheckNumber = int(btoi(data[2:6]))
		out.DateTime = time.Now().Format("2006-01-02 15:04:05")
		if len(data) > 10 {
			out.DateTime = time.Unix(btoi(data[11:16]), 0).Format("2006-01-02 15:04:05")
		}
		out.ShiftState = 1
	}
	/*Запрос денежного регистра
	Команда: 1AH. Длина сообщения: 6 или 7 байт.
	Пароль оператора (4 байта)
	Номер [Ф-]регистра (1 байт) 0… 255 или Номер К-регистра (2 байт) 0…65535
	Ответ: 1AH. Длина сообщения: 9 байт.
	Код ошибки (1 байт)
	Порядковый номер оператора (1 байт) 1…30
	Содержимое регистра (6 байт)
	*/
	tabparam := make([]byte, 7)
	copy(tabparam, admpass)
	for mode := byte(0); mode < 4; mode++ {
		for otd := uint8(0); otd < 16; otd++ {
			//регистры 0-63
			tabparam[4] = mode + otd*4 + 121 //0-приход, 1-расход, 2-возврат прихода, 3-возврат расхода (1 отдел) (4..7 2 отдел и т.д до 60..63 16 отдел)
			errcode, data, err := kkm.SendCommand(0x1a, tabparam[:7])
			if err != nil {
				//log.Printf("kkmCloseShift: %v", err)
				c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if errcode == 0 {
				switch mode {
				case 0:
					out.CountersOperationType1.TotalChecksAmount = out.CountersOperationType1.TotalChecksAmount + float64(btoi(data[1:])/int64(DIGITS))
				case 1:
					out.CountersOperationType2.TotalChecksAmount = out.CountersOperationType2.TotalChecksAmount + float64(btoi(data[1:])/int64(DIGITS))
				case 2:
					out.CountersOperationType3.TotalChecksAmount = out.CountersOperationType3.TotalChecksAmount + float64(btoi(data[1:])/int64(DIGITS))
				case 3:
					out.CountersOperationType4.TotalChecksAmount = out.CountersOperationType4.TotalChecksAmount + float64(btoi(data[1:])/int64(DIGITS))
				}
			}

		}
	}
	//144…147 – количество чеков по 4 типам торговых операций (приход, расход, возврат	прихода, возврат расхода) за смену
	for mode := byte(0); mode < 4; mode++ {
		tabparam[4] = mode + 144
		errcode, data, err := kkm.SendCommand(0x1b, tabparam[:7])
		if err != nil {
			//log.Printf("kkmCloseShift: %v", err)
			c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errcode == 0 {
			switch mode {
			case 0:
				out.CountersOperationType1.CheckCount = int(btoi(data[1:]))
			case 1:
				out.CountersOperationType2.CheckCount = int(btoi(data[1:]))
			case 2:
				out.CountersOperationType3.CheckCount = int(btoi(data[1:]))
			case 3:
				out.CountersOperationType4.CheckCount = int(btoi(data[1:]))
			}
		}
	}
	/*
		120 – наличность в кассе на момент закрытия чека;
		241 – накопление наличности в кассе;
		242 – накопление внесений за смену;
		243 – накопление выплат за смену;
		200 - общее количество чеков коррекции прихода;
		201 - общее количество чеков коррекции расхода;
		202 - количество чеков коррекции прихода за смену;
		203 - количество чеков коррекции расхода за смену;
		4224 – Сумма чеков коррекции прихода;
		4225 – Сумма чеков коррекции расхода*/
	tabparam[4] = 241
	errcode, data, err := kkm.SendCommand(0x1a, tabparam[:7])
	if err != nil {
		//log.Printf("kkmCloseShift: %v", err)
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errcode == 0 {
		out.CashBalance = float64(btoi(data[1:]) / int64(DIGITS))
	}
	tabparam[4] = 200
	errcode, data, err = kkm.SendCommand(0x1a, tabparam[:7])
	if err != nil {
		//log.Printf("kkmCloseShift: %v", err)
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errcode == 0 {
		out.CountersOperationType1.CorrectionCheckCount = int(btoi(data[1:]))
	}
	tabparam[4] = 201
	errcode, data, err = kkm.SendCommand(0x1a, tabparam)
	if errcode == 0 {
		out.CountersOperationType3.CorrectionCheckCount = int(btoi(data[1:]))
	}
	copy(tabparam[4:], itob(4224)[:2])
	errcode, data, err = kkm.SendCommand(0x1a, tabparam)
	if errcode == 0 {
		out.CountersOperationType1.TotalCorrectionChecksAmount = float64(btoi(data[1:]))
	}
	copy(tabparam[4:], itob(4225)[:2])
	errcode, data, err = kkm.SendCommand(0x1a, tabparam)
	if errcode == 0 {
		out.CountersOperationType3.TotalCorrectionChecksAmount = float64(btoi(data[1:]))
	}

	c.XML(http.StatusBadRequest, out)
}
