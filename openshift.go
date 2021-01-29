package main

import (

	//"errors"

	"encoding/xml"
	"log"
	"time"

	"net/http"

	"github.com/gin-gonic/gin"
)

func openShift(c *gin.Context) {
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
		CheckCount                  int     `xml:"CheckCount" binding:"required"`                  //Количество чеков по операции данного типа
		TotalChecksAmount           float64 `xml:"TotalChecksAmount" binding:"required"`           //Итоговая сумма чеков по операциям данного типа
		CorrectionCheckCount        int     `xml:"CorrectionCheckCount" binding:"required"`        //Количество чеков коррекции по операции данного типа
		TotalCorrectionChecksAmount float64 `xml:"TotalCorrectionChecksAmount" binding:"required"` //Итоговая сумма чеков коррекции по операциям данного типа
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
		CountersOperationType4       OperationCounters `xml:"CountersOperationType4" binding:"-"`
		CashBalance                  float64           `xml:"CashBalance,attr" binding:"-"`                  //Остаток наличных денежных средств в кассе
		BacklogDocumentsCounter      int               `xml:"BacklogDocumentsCounter,attr" binding:"-"`      //Количество непереданных документов
		BacklogDocumentFirstNumber   int               `xml:"BacklogDocumentFirstNumber,attr" binding:"-"`   //Номер первого непереданного документа
		BacklogDocumentFirstDateTime string            `xml:"BacklogDocumentFirstDateTime,attr" binding:"-"` //Дата и время первого из непереданных документов
		FNError                      bool              `xml:"FNError,attr" binding:"required"`               //Признак необходимости срочной замены ФН
		FNOverflow                   bool              `xml:"FNOverflow,attr" binding:"required"`            //Признак переполнения памяти ФН
		FNFail                       bool              `xml:"FNFail,attr" binding:"required"`                //Признак исчерпания ресурса ФН
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

	if out.ShiftState != 1 {
		//смена не закрыта, заполним выходные параметры и выходим
		c.XML(http.StatusBadRequest, gin.H{"error": "смена уже открыта"})
		return
	}

	/*Начать открытие смены
	Код команды FF41h . Длина сообщения: 6 байт.
	Пароль системного администратора: 4 байта
	Ответ: FF41h Длина сообщения: 1 байт.
	Код ошибки: 1 байт*/
	//sent tlv
	errcode, _, err = kkm.SendCommand(0xff41, admpass)
	if err != nil {
		log.Printf("kkmOpenShift: %v", err)
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errcode > 0 {
		//старая ккм, просто откроем смену
		errcode, _, err := kkm.SendCommand(0xe0, admpass)
		if err != nil {
			log.Printf("kkmOpenShift: %v", err)
			c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errcode > 0 {
			c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
			return
		}
		//запросим параметры смены
		state := kkm.GetState()
		out.ShiftNumber = int(state.LastSession) + 1
		out.ShiftState = 2
		kkm.SetState(2, state.SubState, state.Flag, state.FlagFP)
	} else {
		//отправим tlv с параметрами и откроем смену ФН
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
		//теперь откроем
		errcode, data, err := kkm.SendCommand(0xff0b, admpass)
		if err != nil {
			log.Printf("kkmOpenShift: %v", err)
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
		out.ShiftState = 2
	}
	//заполним выходные параметры по запросу
	c.XML(http.StatusBadRequest, out)
}
