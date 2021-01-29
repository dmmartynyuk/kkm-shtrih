package main

import (
	"encoding/binary"
	//"errors"
	"flag"
	"kkm-shtrih/drv"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"

	"net/http"

	"github.com/gin-gonic/gin"
	bolt "go.etcd.io/bbolt"
)

//DB указатель на базу данных
var DB *bolt.DB

//Kkm экземпляр драйвера
var Kkm drv.KkmDrv

//KkmServ экземпляр сервера
var KkmServ = Serv{}

//ADMINPASSWORD значение по умолчанию пароль администратора
var ADMINPASSWORD = []byte{0x1e, 0x00, 0x00, 0x00}

//DEFAULTPASSWORD значение по умолчанию для пароля кассира
var DEFAULTPASSWORD = []byte{0x1, 0x0, 0x0, 0x0}

//DEFAULTPORT порт по умолчанию
var DEFAULTPORT = "/dev/ttyUSB0"

//DEFAULTBOD скорость порта по умолчанию
var DEFAULTBOD int64 = 115200

//CODEPAGE Кодировка текста для устройств
var CODEPAGE = "cp1251"

//MAXATTEMPT Кол-во попыток и таймаут
var MAXATTEMPT int64 = 12

//BYTETIMEOUT задержка для получения одного байта порта
var BYTETIMEOUT int64 = 10 //milsec
//PORTTIMEOUT задержка для ккм
var PORTTIMEOUT int64 = 5000 //milsec, 5sec

//LENLINE длина строки по умолчанию
var LENLINE uint8 = 32

//DIGITS разрядность сумм
var DIGITS int = 2

func searchKKM(c *gin.Context) {
	ret := drv.SearchKKM()
	c.JSON(http.StatusOK, gin.H{"error": false, "devices": ret})
}

func getPorts(c *gin.Context) {
	ret, goos := drv.GetPortName()
	c.JSON(http.StatusOK, gin.H{"error": false, "os": goos, "ports": ret})
}

func runCommand(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}

	cmd := c.Param("command")
	if cmd == "" {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "нет комманды"})
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

	strparams := c.QueryMap("params")
	params := make([]byte, 0, 64)
	//первым идет пароль, 4 байта, потом все остальные по-порядку
	for i := 0; i < len(strparams); i++ {
		if v, ok := strparams[strconv.FormatInt(int64(i), 10)]; ok {
			vint, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": "параметры комманды не верны"})
				return
			}
			res := itob(vint)
			if i == 0 {
				params = append(params, res[:4]...)
			} else {
				params = append(params, res[0])
			}
		}

	}
	_, err = kkm.Connect()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	defer kkm.Close()
	kkmerr, data, descr, err := kkmRunFunction(kkm, cmd, params)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	sdata := make([]string, len(data))
	for i := 0; i < len(data); i++ {
		sdata[i] = strconv.FormatUint(uint64(data[i]), 10)
	}
	hdata["deviceID"] = deviceID
	hdata["kkmerr"] = kkmerr
	hdata["retdata"] = sdata
	hdata["resdescr"] = descr
	hdata["error"] = false
	c.JSON(http.StatusOK, hdata)

}

func getDataKKT(c *gin.Context) {
	type TableParametersKKT struct {
		KKTNumber       string `xml:"kktnumber" json:"kktnumber" binding:"-"`                    //Регистрационный номер ККТ
		KKTSerialNumber string `xml:"kktserialnumber" json:"kktserialnumber" binding:"required"` //Заводской номер ККТ
		FirmwareVersion string `xml:"FirmwareVersion" json:"FirmwareVersion" binding:"-"`        //Версия прошивки
		Fiscal          bool   `xml:"Fiscal" json:"Fiscal" binding:"required"`                   //Признак регистрации фискального накопителя
		FFDVersionFN    string `xml:"FFDVersionFN" json:"FFDVersionFN" binding:"required"`       //Версия ФФД ФН (одно из следующих значений "1.0","1.1")
		FFDVersionKKT   string `xml:"FFDVersionKKT" json:"FFDVersionKKT" binding:"required"`     //Версия ФФД ККТ (одно из следующих значений "1.0","1.0.5","1.1")
		FNSerialNumber  string `xml:"FNSerialNumber" json:"FNSerialNumber" binding:"required"`   //Заводской номер ФН
		DocumentNumber  string `xml:"DocumentNumber" json:"DocumentNumber" binding:"-"`          //Номер документа регистрация фискального накопителя
		DateTime        string `xml:"DateTime" json:"DateTime" binding:"-"`                      //Дата и время операции регистрации фискального накопителя
		CompanyName     string `xml:"CompanyName" json:"CompanyName" binding:"-"`                //Название организации
		INN             string `xml:"INN" json:"INN" binding:"-"`                                //ИНН организация
		SaleAddress     string `xml:"SaleAddress" json:"SaleAddress" binding:"-"`                //Адрес проведения расчетов
		SaleLocation    string `xml:"SaleLocation" json:"SaleLocation" binding:"-"`              //Место проведения расчетов
		TaxationSystems string `xml:"TaxationSystems" json:"TaxationSystems" binding:"-"`        //Коды системы налогообложения через разделитель ",".
		//Коды системы налогообложения приведены в таблице "Системы налогообложения".
		IsOffline   bool   `xml:"IsOffline" json:"IsOffline" binding:"-"`     //Признак автономного режима
		IsEncrypted bool   `xml:"IsEncrypted" json:"IsEncrypted" binding:"-"` //Признак шифрование данных
		IsService   bool   `xml:"IsService" json:"IsService" binding:"-"`     //Признак расчетов за услуги
		IsExcisable bool   `xml:"IsExcisable" json:"IsExcisable" binding:"-"` //Продажа подакцизного товара
		IsGambling  bool   `xml:"IsGambling" json:"IsGambling" binding:"-"`   //Признак проведения азартных игр
		IsLottery   bool   `xml:"IsLottery" json:"IsLottery" binding:"-"`     //Признак проведения лотереи
		AgentTypes  string `xml:"AgentTypes" json:"AgentTypes" binding:"-"`   //Коды признаков агента через разделитель ",".
		//Коды приведены в таблице 10 форматов фискальных данных.
		BSOSing            bool   `xml:"BSOSing" json:"BSOSing" binding:"-"`                       //Признак формирования АС БСО
		IsOnlineOnly       bool   `xml:"IsOnlineOnly" json:"IsOnlineOnly" binding:"-"`             //Признак ККТ для расчетов только в Интернет
		IsAutomaticPrinter bool   `xml:"IsAutomaticPrinter" json:"IsAutomaticPrinter" binding:"-"` //Признак установки принтера в автомате
		IsAutomatic        bool   `xml:"IsAutomatic" json:"IsAutomatic" binding:"-"`               //Признак автоматического режима
		AutomaticNumber    string `xml:"AutomaticNumber" json:"AutomaticNumber" binding:"-"`       //Номер автомата для автоматического режима
		OFDCompany         string `xml:"OFDCompany" json:"OFDCompany" binding:"-"`                 //Название организации ОФД
		OFDCompanyINN      string `xml:"OFDCompanyINN" json:"OFDCompanyINN" binding:"-"`           //ИНН организации ОФД
		FNSURL             string `xml:"FNSURL" json:"FNSURL" binding:"-"`                         //Адрес сайта уполномоченного органа (ФНС) в сети «Интернет»
		SenderEmail        string `xml:"SenderEmail" json:"SenderEmail" binding:"-"`
		ErrState           uint8  `xml:"errstate" json:"errstate" binding:"-"`
		ErrMsg             string `xml:"errmessage" json:"errmessage" binding:"-"`
	}
	var res TableParametersKKT
	res.ErrState = 0
	res.ErrMsg = ""

	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"errstate": 1, "errmessage": "deviceID не зарегистрирован"})
		return
	}
	/*
		procID := c.DefaultQuery("proc","0")
		pid, err:= strconv.Atoi(procID)
		if err !=nil {
			pid=0
		}*/
	if kkm.ChkBusy(0) {
		c.XML(http.StatusOK, gin.H{"errstate": 1, "errmessage": "ККТ занята"})
		return
	}
	//займем ккм
	procid := int(time.Now().Unix())
	kkm.SetBusy(procid)
	//освободим по завершению
	defer kkm.SetBusy(0)

	admpass := kkm.GetAdminPass()

	errcode, err := kkm.FNGetStatus()
	if err != nil {
		c.XML(http.StatusOK, gin.H{"errstate": 1, "errmessage": err.Error()})
		return
	}

	kkmParam := kkm.GetParam()

	//запрос состояния ккм
	errcode, data, err := kkm.SendCommand(0x11, admpass)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"errstate": 1, "errmessage": err.Error()})
		return
	}
	if errcode > 0 {
		c.XML(http.StatusOK, gin.H{"errstate": errcode, "errmessage": kkm.ParseErrState(errcode)})
		return
	}
	kkm.SetState(data[13], data[14], binary.LittleEndian.Uint16(data[11:13]), data[29])

	res.KKTSerialNumber = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(data[30:34])), 10)

	res.KKTNumber = kkmParam.KKMRegNumber
	res.FirmwareVersion = string(data[1]) + "." + string(data[2]) //Версия ПО ККТ
	res.FFDVersionKKT = string(data[16]) + "." + string(data[17]) // string Версия ФФД ККТ (одно из следующих значений "1.0","1.0.5","1.1")
	res.CompanyName = kkmParam.Fname                              //  string Название организации
	inn := make([]byte, 8)
	copy(inn, data[40:46])
	res.INN = strconv.FormatUint(uint64(binary.LittleEndian.Uint64(inn)), 10) // string ИНН организация

	//запрос состояния FN
	errcode, data, err = kkm.SendCommand(0xFF01, admpass)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"errstate": 1, "errmessage": err.Error()})
		return
	}
	if errcode > 0 {
		res.Fiscal = false
		res.ErrState = errcode
		res.ErrMsg = kkm.ParseErrState(errcode)
		//	c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		//	return
	} else {
		/*	Запрос статуса ФН
			Код команды FF01h.
			Ответ: Длина сообщения: 31 байт.
			Состояние фазы жизни: 1 байт
			Бит 0 – проведена настройка ФН
			Бит 1 – открыт фискальный режим
			Бит 2 – закрыт фискальный режим
			Бит 3 – закончена передача фискальных данных в ОФД
		*/
		res.Fiscal = (data[0]&0b0010) > 0 && ((data[0] & 0b0100) == 0) // bool Признак регистрации фискального накопителя
	}
	//Запрос итогов последней фискализации (перерегистрации)
	errcode, data, err = kkm.SendCommand(0xFF09, admpass)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"errstate": 1, "errmessage": err.Error()})
		return
	}
	if errcode > 0 {
		if res.ErrState == 0 {
			res.ErrMsg = kkm.ParseErrState(errcode)
			res.ErrState = errcode
		}
		//c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		//return
	} else {
		/*Ответ для ФФД 1.0 и 1.5: FF09h Длина сообщения: 48(53) байт - 3байта (FF09h+err)
		Дата и время: 5 байт DATE_TIME   (0:5)
		ИНН : 12 байт ASCII		(5:17)
		Регистрационный номер ККT: 20 байт ASCII  (17:27)
		Код налогообложения: 1 байт  (27)
		Бит 0 – ОСН
		Бит 1 – УСН доход
		Бит 2 – УСН доход минус расход
		Бит 3 – ЕНВД
		Бит 4 – ЕСП
		Бит 5 – ПСН
		Режим работы: 1 байт (28)
		Бит 0 – Шифрование
		Бит 1 – Автономный режим
		Бит 2 – Автоматический режим
		Бит 3 – Применение в сфере услуг
		Бит 4 – Режим БСО
		Бит 5 – Применение в Интернет
		Номер ФД: 4 байта  (33:37)
		Фискальный признак: 4 байта  (41:45)
		Дата и время: 5 байт DATE_TIME  (45:50)

		Ответ для ФФД 1.1:FF09h Длина сообщения: 65 байт-3 байта (FF09h+err)
		Расширенные признаки работы ККТ: 1 байт (29:30)
		ИНН ОФД: 12 байт ASCII  (30:42)
		Код причины изменения сведений о ККТ:4 байта  (42:46)
		Номер ФД: 4 байта  (46:50)
		Фискальный признак: 4 байта (50:54)
		*/
		if len(data) < 52 {
			res.FFDVersionFN = "1.0" //  string Версия ФФД ФН (одно из следующих значений "1.0","1.1")
		} else {
			res.FFDVersionFN = "1.1"
		}
		res.DocumentNumber = string(data[33:37]) // string Номер документа регистрация фискального накопителя

		res.DateTime = strconv.FormatUint(uint64(data[0]), 10) + "." + strconv.FormatUint(uint64(data[1]), 10) + "." + strconv.FormatUint(uint64(data[2]), 10) + " " + strconv.FormatUint(uint64(data[3]), 10) + ":" + strconv.FormatUint(uint64(data[4]), 10) // string Дата и время операции регистрации фискального накопителя

		tax := ""
		comma := ""
		for i := int64(0); i < 5; i++ {
			mask := byte(1)
			if (data[27] & mask) > 0 {
				tax = tax + comma + strconv.FormatInt(i, 10)
				comma = ","
			}
			mask = mask << 1
		}
		res.TaxationSystems = tax // string Коды системы налогообложения через разделитель ",".
		//Коды системы налогообложения 0-Общая,1-Упрощенная (Доход),2-Упрощенная (Доход минус Расход),3-Енвд,4-Единый сельхоз налог,5-Патентная система налогообложения.
		res.IsOffline = (data[28] & 0b0010) > 0   // bool  Признак автономного режима
		res.IsEncrypted = (data[28] & 0b0001) > 0 // bool Признак шифрование данных
		res.IsService = (data[28] & 0b001000) > 0 // bool  Признак расчетов за услуги

		//Коды приведены в таблице 10 форматов фискальных данных.
		res.BSOSing = (data[28] & 0b010000) > 0      //    bool   Признак формирования АС БСО
		res.IsOnlineOnly = (data[28] & 0b100000) > 0 //   bool   Признак ККТ для расчетов только в Интернет
		res.IsAutomatic = (data[28] & 0b0000100) > 0 //  bool   Признак автоматического режима
	}
	/*
			Чтение таблицы
		Команда: 1FH. Длина сообщения: 9 байт.
		Пароль системного администратора (4 байта)
		Таблица (1 байт)
		Ряд (2 байта)
		Поле (1 байт)
		Ответ: 1FH. Длина сообщения: (2+X) байт.
		Код ошибки (1 байт)
		Значение (X байт) до 40 или до 2461
		байт
	*/
	tabparam := make([]byte, 8)
	copy(tabparam, admpass)
	//binary.LittleEndian.PutUint16(b, uint16(18))
	tabparam[4] = 18 //таблица 18 Fiscal storage
	tabparam[5] = 0  //ряд
	tabparam[6] = 1  //ряд
	tabparam[7] = 21 //поле
	errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
	if errcode > 0 {
		if res.ErrState == 0 {
			res.ErrMsg = kkm.ParseErrState(errcode)
			res.ErrState = errcode
		}
		//c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ParseErrState(errcode)})
		//return
	} else {
		res.IsExcisable = (data[0] & 0b0001) > 0        // bool Продажа подакцизного товара
		res.IsGambling = (data[0] & 0b0010) > 0         // bool Признак проведения азартных игр
		res.IsLottery = (data[0] & 0b0100) > 0          // bool Признак проведения лотереи
		res.IsAutomaticPrinter = (data[0] & 0b1000) > 0 // bool  Признак установки принтера в автомате

		tabparam[7] = 4 //Fs serial number
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.FNSerialNumber = string(decodeWindows1251(data)) // string Заводской номер ФН

		tabparam[7] = 9 //поле address kkm 128 byte
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.SaleAddress = string(decodeWindows1251(data)) // string Адрес проведения расчетов
		tabparam[7] = 18                                  //поле address 2 kkm 128 byte
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.SaleAddress = res.SaleAddress + string(decodeWindows1251(data))
		tabparam[7] = 14 //поле место расчета kkm 128 byte
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.SaleLocation = string(decodeWindows1251(data)) // string Место проведения расчетов
		tabparam[7] = 20                                   //поле место расчета 2 kkm 128 byte
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.SaleLocation = res.SaleLocation + string(decodeWindows1251(data))
		tabparam[7] = 16 //поле признак агента
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		agent := ""
		comma := ""
		for i := int64(0); i < 7; i++ {
			mask := byte(1)
			if (data[0] & mask) > 0 {
				agent = agent + comma + strconv.FormatInt(i, 10)
				comma = ","
			}
			mask = mask << 1
		}
		res.AgentTypes = agent // string Коды признаков агента через разделитель ",".
		//0-«БАНК. ПЛ. АГЕНТ»,1-«БАНК. ПЛ. СУБАГЕНТ»,2-ПЛ. АГЕНТ,3-ПЛ. СУБАГЕНТ,4-ПОВЕРЕННЫЙ,5-КОМИССИОНЕР,6-АГЕНТ

		tabparam[7] = 12 //поле инн офд
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		copy(inn, data[:])
		res.OFDCompanyINN = strconv.FormatUint(uint64(binary.LittleEndian.Uint64(inn)), 10) //  string ИНН организации ОФД

		tabparam[7] = 10 //поле имя офд
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.OFDCompany = string(decodeWindows1251(data)) // string Название организации ОФД

		tabparam[7] = 13 //поле email
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.FNSURL = string(decodeWindows1251(data)) //  string Адрес сайта уполномоченного органа (ФНС) в сети «Интернет»

		tabparam[7] = 15 //поле email
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.SenderEmail = string(decodeWindows1251(data)) //  string

		tabparam[7] = 15 //поле email
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.SenderEmail = string(decodeWindows1251(data))

		//Таблица 24 Встраиваемая интернет техника
		tabparam[4] = 24
		tabparam[7] = 1 //поле Заводской номер автомата
		errcode, data, err = kkm.SendCommand(0x1F, tabparam[:])
		res.AutomaticNumber = string(decodeWindows1251(data)) //  string Номер автомата для автоматического режима
	}
	c.XML(http.StatusOK, res)
	return
}

func getServSetting(c *gin.Context) {
	hdata := make(map[string]interface{})
	//прочитаем настройки сервера
	kkms := KkmServ.GetKeys()
	for _, key := range kkms {
		d, err := KkmServ.GetDrv(key)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
			return
		}
		hdata[key] = d.GetStruct()
	}
	hdata["deviceids"] = kkms
	hdata["error"] = false
	c.JSON(http.StatusOK, hdata)
}

func getParamKKT(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	hdata[deviceID] = kkm.GetStruct()
	c.JSON(http.StatusOK, hdata)
}

//setServSetting установка параметров сервера
func setServSetting(c *gin.Context) {
	//hdata := make(map[string]interface{})
	//заполним Kkmserv из базы
	var jkkm drv.KkmDrvSer
	if err := c.ShouldBindJSON(&jkkm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}

	err := KkmServ.SetServ(&jkkm)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	getServSetting(c)
	//hdata["device"] = jkkm
	//hdata["error"] = false
	//c.JSON(http.StatusOK, hdata)
}

//indexPage начальная страница приложения
func indexPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	//заполним Kkmserv из базы

	hdata["page"] = "index"

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"index.html",
		// Передадим данные в шаблон
		hdata,
	)
}

//editPage редактирование параметров ккм
func editPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	//заполним Kkmserv из базы

	hdata["page"] = "edit"

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"edit.html",
		// Передадим данные в шаблон
		hdata,
	)
}

//commandPage редактирование параметров ккм
func commandPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	//заполним Kkmserv из базы

	hdata["page"] = "command"

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"command.html",
		// Передадим данные в шаблон
		hdata,
	)
}

//initDB в пустой базе создает и записывает значения по умолчанию
func initDB() error {
	return DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("DefaultConfig"))
		if err != nil {
			return err
		}
		//var kkm = drv.KkmDrv{}
		/*
			copy(admpass, ADMINPASSWORD)
			copy(admpass, DEFAULTPASSWORD)
			kkm.MaxAttemp = MAXATTEMPT
			kkm.TimeOut = PORTTIMEOUT
			kkm.Opt.Baud = int(DEFAULTBOD)
			kkm.Opt.Name = DEFAULTPORT
			kkm.Opt.ReadTimeout = time.Duration(BYTETIMEOUT) * time.Millisecond
		*/
		err = b.Put([]byte("AdminPassword"), ADMINPASSWORD)
		err = b.Put([]byte("DefaultPassword"), DEFAULTPASSWORD)
		err = b.Put([]byte("DefaultPort"), []byte(DEFAULTPORT))
		err = b.Put([]byte("DefaultBod"), itob((DEFAULTBOD)))
		err = b.Put([]byte("MaxAttempt"), itob(MAXATTEMPT))

		b, err = tx.CreateBucket([]byte("Drivers"))
		uuidWithHyphen := uuid.New()
		kkm := KkmServ.New(uuidWithHyphen.String())
		v, err := kkm.Serialize()
		if err != nil {
			return err
		}
		//uuid := strings.Replace(uuidWithHyphen.String(), "-", "", -1)
		err = b.Put([]byte(kkm.DeviceID), v)
		return nil
	})
}

func main() {
	// открываем базу и считываем конфиг
	// Open the my.db data file in your current directory.
	// It will be created if it doesn't exist
	//port := flag.String("port", "3000", "Номер порта")
	port := flag.Int("port", 3000, "Номер порта")
	portstr := ":" + strconv.Itoa(*port)
	flag.Parse()
	//portstr := ":" + strconv.Itoa(*port)
	var err error
	DB, err = bolt.Open("kkm.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)

	}
	defer DB.Close()

	err = initDB()
	if err != nil && err != bolt.ErrBucketExists {
		log.Fatal(err)
	}

	//init KkmServ
	err = KkmServ.InitServ()
	if err != nil {
		log.Fatal(err)
	}

	router := gin.Default()
	router.Static("/assets", "./assets")
	router.LoadHTMLGlob("./tpl/*")
	//начальная страница настройки
	router.GET("/", indexPage)
	router.GET("/edit", editPage)
	router.GET("/command", commandPage)
	api := router.Group("/api/")
	{
		//service
		api.GET("SearchKKM/", searchKKM)
		api.GET("getPorts/", getPorts)
		api.GET("GetServSetting/", getServSetting)
		api.PUT("SetServSetting/", setServSetting)
		api.POST("run/:DeviceID/:command", runCommand)
		api.GET("GetParamKKT/:DeviceID", getParamKKT)

		//функции для печати
		api.PUT("SetBusy/:DeviceID", setBusy)
		api.PUT("Release/:DeviceID", release)
		api.POST("OpenCheck/:DeviceID", openCheck)
		api.POST("FNOperation/:DeviceID", fnOperation)
		api.POST("PrintString/:DeviceID", printString)
		api.POST("CancelCheck/:DeviceID", cancelCheck)
		api.POST("CloseCheck/:DeviceID", closeCheck)
		api.POST("FNSendTagOperation/:DeviceID", fnSendTagOperation)
		api.POST("FNSendTag/:DeviceID", fnSendTag)

		//1c spec
		api.POST("GetDataKKT/:DeviceID", getDataKKT)
		//api.POST("OperationFN/:DeviceID", OperationFN) //Операция с фискальным накопителем.
		api.POST("OpenShift/:DeviceID", openShift)
		api.POST("CloseShift/:DeviceID", CloseShift)
		api.POST("ProcessCheck/:DeviceID", ProcessCheck)
		//api.POST("ProcessCorrectionCheck/:DeviceID", ProcessCorrectionCheck)
		//api.POST("PrintTextDocument/:DeviceID", PrintTextDocument)
		//api.POST("CashInOutcome/:DeviceID", CashInOutcome)
		//api.POST("PrintXReport/:DeviceID", PrintXReport)
		//api.POST("PrintCheckCopy/:DeviceID", PrintCheckCopy)
		//api.POST("GetCurrentStatus/:DeviceID", GetCurrentStatus)
		//api.POST("ReportCurrentStatusOfSettlements/:DeviceID", ReportCurrentStatusOfSettlements)
		api.POST("OpenCashDrawer/:DeviceID", OpenCashDrawer)
		//api.POST("GetLineLength/:DeviceID", GetLineLength)
		//api.POST("CashInOutcome/:DeviceID", CashInOutcome)

	}
	router.Run(portstr)
}
