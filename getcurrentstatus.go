package main

import (
	"encoding/xml"
	//"log"
	"time"

	"net/http"

	"github.com/gin-gonic/gin"
)

//getCurrentStatus печать х-отчет
func getCurrentStatus(c *gin.Context) {
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
	/*Запрос параметров текущей смены
	Код команды FF40h . Длина сообщения: 6 байт.
	Пароль системного администратора: 4 байта
	Ответ: FF40h Длина сообщения: 6 байт.
	Код ошибки: 1 байт
	Состояние смены: 1 байт [0]
	Номер смены : 2 байта  [1:3]
	Номер чека: 2 байта	[3:]
	*/
	errcode, data, err := kkm.SendCommand(0xff40, admpass)
	if errcode > 0 {
		c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
		return
	}
	out.ShiftNumber = int(btoi(data[1:3]))
	//Номер ФД :4 байта
	out.CheckNumber = int(btoi(data[3:5]))
	out.DateTime = time.Now().Format("2006-01-02 15:04:05")
	out.ShiftState = int(data[0])

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
	errcode, data, err = kkm.SendCommand(0x1a, tabparam[:7])
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
	/*Получить статус информационного обмена
	Код команды FF39h . Длина сообщения: 6 байт.
	Пароль системного администратора: 4 байта
	Ответ: FF39h Длина сообщения: 14 байт.
	Код ошибки: 1 байт
	Статус информационного обмена: 1 байт (0 – нет, 1 – да) [0]
	Бит 0 – транспортное соединение установлено
	Бит 1 – есть сообщение для передачи в ОФД
	Бит 2 – ожидание ответного сообщения (квитанции) от ОФД
	Бит 3 – есть команда от ОФД
	Бит 4 – изменились настройки соединения с ОФД
	Бит 5 – ожидание ответа на команду от ОФД
	Состояние чтения сообщения: 1 байт (1 – да, 0 –нет)	[1]
	Количество сообщений для ОФД: 2 байта	[2:4]
	Номер документа для ОФД первого в очереди: 4 байта [4:8]
	Дата и время документа для ОФД первого в очереди: 5 бай [8:13]
	*/
	errcode, _, err = kkm.SendCommand(0xff39, admpass)
	if errcode > 0 {
		c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
		return
	}
	/*Количество непереданных документов
	BacklogDocumentsCounter int `xml:"BacklogDocumentsCounter,attr" binding:"-"`
	//Номер первого непереданного документа
	BacklogDocumentFirstNumber int `xml:"BacklogDocumentFirstNumber,attr" binding:"-"`
	//Дата и время первого из непереданных документов
	BacklogDocumentFirstDateTime string `xml:"BacklogDocumentFirstDateTime,attr" binding:"-"`*/
	out.BacklogDocumentsCounter = int(btoi(data[2:4]))
	out.BacklogDocumentFirstNumber = int(btoi(data[4:8]))
	out.BacklogDocumentFirstDateTime = time.Unix(btoi(data[8:13]), 0).Format("2006-01-02 15:04:05")
	c.XML(http.StatusBadRequest, out)
}
