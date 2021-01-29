package main

import (

	//"errors"
	"encoding/xml"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

//ProcessCheck печать чека
func ProcessCheck(c *gin.Context) {
	//продажа маркированного товара https://infostart.ru/1c/articles/1192569/
	/*
				<CheckPackage>
				    <Parameters PaymentType="1" SenderEmail="info@1c.ru" CustomerEmail="alex2000@mail.ru" CustomerPhone="" AgentCompensation="" AgentPhone=""/>
				    <Positions>
				        <FiscalString Name="Макароны" Quantity="1" Price="16.75" Amount="16.75" Tax="10"/>
				        <FiscalString Name="Томатный сок" Quantity="1" Price="200" Amount="200" Tax="18"/>
				        <FiscalString Name="Алкоголь Шампрео 0.7" Quantity="1" Price="455" Amount="455" Tax="18"/>
				        <TextString Text="Дисконтная карта: 00002345"/>
				        <Barcode BarcodeType="EAN13" Barcode="2000021262157"/>
				    </Positions>
				    <Payments Cash="471.75" CashLessType1="0" CashLessType2="0" CashLessType3="200"/>
				</CheckPackage>

				Сначала в таблице 17 поле 14 (Печать реквизита пользователя) надо поставить 1.
				Надо передавать тег 15000 (15 тысяч). Тип строка, Это внутренний тег, значение тега, т.е. текст который передадите, попадет в тег
				1086,ЗНАЧ. ДОП. РЕКВИЗИТ. ПОЛЬЗОВ.

				Тег 1085,НАИМЕН. ДОП. РЕКВИЗИТ. ПОЛЬЗОВ. - берется из таблицы 17 поле 13.
				В итоге чек в ФН получается такой:
				.....
				1054,ПРИЗН. РАСЧЕТА:1 (Приход)
				1020,ИТОГ:1200.00
				1084,ДОП. РЕКВИЗИТ ПОЛЬЗОВ.
				 1085,НАИМЕН. ДОП. РЕКВИЗИТ. ПОЛЬЗОВ.:Код
				 1086,ЗНАЧ. ДОП. РЕКВИЗИТ. ПОЛЬЗОВ.:Дополнительныйрекв
				1059,ПРЕДМ. РАСЧЕТА
				1030,НАИМЕН. ПРЕДМ. РАСЧЕТА:1
				1079,ЦЕНА ЗА ЕД. ПРЕДМ. РАСЧ.:1200.00
				1023,КОЛ-ВО ПРЕДМ. РАСЧЕТА:1.000000

				боевой код маркировки для обуви:
		010465012500817521dqT2iTkBxEwxs240640491ffd092OTMWE7tASOzx1G7dXWcZgM7VICsC8W5g5CVEVr69VqI6dfkyMoUcJ6OhV63wMC5oFnBmdO8tNFqjd8vjDvVXCg==
		Здесь "" и есть тот символ, который необходимо заменить в утилите "Тест драйвера ФР"  на <0x1D>, т.е. для передачи через утилиту при помощи команды FNSendItemBarcode необходимо использовать строку вида:
		010465012500817521dqT2iTkBxEwxs<0x1D>240640491ffd0<0x1D>92OTMWE7tASOzx1G7dXWcZgM7VICsC8W5g5CVEVr69VqI6dfkyMoUcJ6OhV63wMC5oFnBmdO8tNFqjd8vjDvVXCg==

		P.S. Символ  "" - в интерпретации ASCII, в зависимости от кодировки, может иметь значение: "1D", "\u001D" или "&#x001D".
	*/

	type CorrectionData struct {
		Type        int    `xml:"Type" binding:"required"`        //Тип коррекции 0 - самостоятельно 1 - по предписанию
		Description string `xml:"Description" binding:"required"` //Описание коррекции
		Datе        string `xml:"Datе" binding:"required"`        //datetime	Дата совершения корректируемого расчета
		Number      string `xml:"Number" binding:"required"`      //Номер предписания налогового органа
	}
	type AgentData struct {
		//	Операция платежного агента
		AgentOperation string `xml:"AgentOperation,attr" binding:"-"`
		//Телефон платежного агента. Допустимо несколько значений через разделитель ",".
		AgentPhone string `xml:"AgentPhone,attr" binding:"-"`
		//Телефон оператора по приему платежей. Допустимо несколько значений через разделитель ",".
		PaymentProcessorPhone string `xml:"PaymentProcessorPhone,attr" binding:"-"`
		//Телефон оператора перевода. Допустимо несколько значений через разделитель ",".
		AcquirerOperatorPhone string `xml:"AcquirerOperatorPhone,attr" binding:"-"`
		//Наименование оператора перевода
		AcquirerOperatorName string `xml:"AcquirerOperatorName,attr" binding:"-"`
		//Адрес оператора перевода
		AcquirerOperatorAddress string `xml:"AcquirerOperatorAddress,attr" binding:"-"`
		//ИНН оператора перевода
		AcquirerOperatorINN string `xml:"AcquirerOperatorINN,attr" binding:"-"`
	}
	type VendorData struct {
		VendorPhone string `xml:"VendorPhone,attr" binding:"-"` //Телефон поставщика. Допустимо несколько значений через разделитель ",".
		VendorName  string `xml:"VendorName,attr" binding:"-"`  //Наименование поставщика
		VendorINN   string `xml:"VendorINN,attr" binding:"-"`   //ИНН поставщика
	}
	type GoodCodeData struct {
		MarkingCode string `xml:"MarkingCode,attr" binding:"required"` //Значение реквизита кода товара (Значение тэга 1162). Кодируется текстом в кодировке Base64.
	}
	type UserAttribute struct {
		Name  string `xml:"Name" binding:"required"`  //Имя реквизита
		Value string `xml:"Value" binding:"required"` //Значение реквизита
	}
	type Parameters struct {
		CashierName   string `xml:"CashierName,attr" binding:"required"` //ФИО и должность уполномоченного лица для проведения операции	Формирование нового чека с заданным атрибутами. При формирование чека ККТ должен проверять, что передаваемый код системы налогообложения доступен для данного фискализированного ФН.
		CashierINN    string `xml:"CashierINN,attr" binding:"-"`         //ИНН уполномоченного лица для проведения операции
		OperationType int    `xml:"OperationType,attr" binding:"-"`      //Тип операции (Таблица 25 документа ФФД):
		//аналог OperationType
		//1 - приход денежных средств
		//2 - возврат прихода денежных средств
		//3 - расход денежных средств
		//4 - возврат расхода денежных средств
		PaymentType int `xml:"PaymentType,attr" binding:"-"`
		//Код системы налогообложения. Коды системы налогообложения приведены в таблице "Системы налогообложения".
		//0	Общая
		//1	Упрощенная (Доход)
		//2	Упрощенная (Доход минус Расход)
		//3	Единый налог на вмененный доход
		//4	Единый сельскохозяйственный налог
		//5	Патентная система налогообложения
		TaxationSystem int `xml:"TaxationSystem,attr" binding:"required"`
		//Покупатель (клиент) - наименование организации или фамилия, имя, отчество (при наличии), серия и номер паспорта покупателя (клиента).
		CustomerInfo string `xml:"CustomerInfo,attr" binding:"-"`
		//ИНН организации или покупателя (клиента)
		CustomerINN string `xml:"CustomerINN,attr" binding:"-"`
		//Email покупателя (клиента)
		CustomerEmail string `xml:"CustomerEmail,attr" binding:"-"`
		//Телефонный номер покупателя (клиента)
		CustomerPhone       string                            `xml:"CustomerPhone,attr" binding:"-"`
		SenderEmail         string                            `xml:"SenderEmail,attr" binding:"-"`         //Адрес электронной почты отправителя чека
		SaleAddress         string                            `xml:"SaleAddress,attr" binding:"-"`         //Адрес проведения расчетов
		SaleLocation        string                            `xml:"SaleLocation,attr" binding:"-"`        //Место проведения расчетов
		AgentType           int                               `xml:"AgentType,attr" binding:"-"`           //Признак агента. См. таблицу "Признаки агента"
		AdditionalAttribute string                            `xml:"AdditionalAttribute,attr" binding:"-"` //Дополнительный реквизит чека
		AgentData           `xml:"AgentData" binding:"-"`     //Вложенная структура	Данные агента
		VendorData          `xml:"VendorData" binding:"-"`    //Вложенная структура	Данные поставщика
		UserAttribute       `xml:"UserAttribute" binding:"-"` //Вложенная структура	Дополнительный реквизит пользователя
		CorrectionData      CorrectionData                    `xml:"CorrectionData" binding:"-"` //Да* Вложенная структура	Данные по операции коррекции.Данное поле обязательно только для чека коррекции.
	}

	type Barcode struct {
		Type string `xml:"Type" binding:"-"`
		//Значение штрихкода.
		ValueBase64 string `xml:"ValueBase64,attr" binding:"-"`
	}

	type FiscalString struct {
		//Наименование товара	Регистрирует фискальную строку с переданными реквизитами.
		//При печати длинных фискальных строк необходимо делать перенос на следующую строку.
		Name string `xml:"Name,attr" binding:"required"`
		//Количество товара
		Quantity float64 `xml:"Quantity,attr" binding:"required"`
		//Цена единицы товара с учетом скидок/наценок
		PriceWithDiscount float64 `xml:"PriceWithDiscount,attr" binding:"required"`
		//Конечная сумма по предмету расчета с учетом всех скидок/наценок
		AmountWithDiscount float64 `xml:"AmountWithDiscount,attr" binding:"required"`
		DiscountAmount     float64 `xml:"DiscountAmount,attr" binding:"-"` //Сумма скидок и наценок (если значение > 0 то в чеке выводиться скидка, если значение < 0 то наценка
		Department         int     `xml:"Department,attr" binding:"-"`     //Отдел, по которому ведется продажа
		VATRate            string  `xml:"VATRate,attr" binding:"required"` //Ставка НДС:
		//"none" - БЕЗ НДС
		//"20" - НДС 20
		//"18" - НДС 18
		//"10" - НДС 10
		//"0" - НДС 0
		//"20/120" - расчетная ставка 20/120
		//"18/118" - расчетная ставка 18/118
		//"10/110" - расчетная ставка 10/110

		//Сумма НДС за предмет расчета.
		//В ККТ должен быть отключен расчет налогов, и в чеке выводиться сумма НДС рассчитанная в 1С.
		//Итоговые суммы НДС по чеку должны рассчитывать по строкам.
		VATAmount float64 `xml:"VATAmount,attr" binding:"-"`
		//Признак способа расчета. См. таблицу "Признаки способа расчета" Признаки способа расчета
		//Код	Описание
		//1	Предоплата полная
		//2	Предоплата частичная
		//3	Аванс
		//4	Полный расчет
		//5	Частичный расчет и кредит
		//6	Передача в кредит
		//7	Оплата кредита
		PaymentMethod int `xml:"PaymentMethod,attr" binding:"-"`
		//Признак предмета расчета. См. таблицу "Признаки предмета расчета" Признаки предмета расчета
		//1	Товар, 2	Подакцизный товар, 3	Работа, 4	Услуга, 5	Ставка азартной игры
		//6	Выигрыш азартной игры, 7	Лотерейный билет, 8	Выигрыш лотереи, 9	Предоставление результатов интеллектуальной деятельности
		//10	Платеж,	11	Агентское вознаграждение, 12	Выплата, 13	Иной предмет расчета, 14	Имущественное право
		//15	Внереализационный доход, 16	Страховые взносы, 17	Торговый сбор, 18	Курортный сбор
		//19	Залог,20	Расход,	21	Взносы на обязательное пенсионное страхование ИП,	22	Взносы на обязательное пенсионное страхование,		23	Взносы на обязательное медицинское страхование ИП
		//24	Взносы на обязательное медицинское страхование,	25	Взносы на обязательное социальное страхование,		26	Платеж казино
		CalculationSubject int `xml:"CalculationSubject,attr" binding:"-"`
		//Признак агента по предмету расчета См. таблицу "Признаки агента по предмету расчета"
		CalculationAgent int `xml:"CalculationAgent,attr" binding:"-"`
		//Вложенная структура	Данные агента
		AgentData AgentData `xml:"AgentData" binding:"-"`
		//	Вложенная структура	Данные поставщика
		VendorData VendorData `xml:"VendorData" binding:"-"`
		//Единица измерения предмета расчета
		MeasurementUnit     string       `xml:"MeasurementUnit,attr" binding:"-"`
		GoodCodeData        GoodCodeData `xml:"GoodCodeData" binding:"-"`             //Вложенная структура	Данные кода товарной номенклатуры
		CountryOfOrigin     string       `xml:"CountryOfOrigin,attr" binding:"-"`     //Цифровой код страны происхождения товара в соответствии с Общероссийским классификатором стран мира
		CustomsDeclaration  string       `xml:"CustomsDeclaration,attr" binding:"-"`  //Регистрационный номер таможенной декларации
		AdditionalAttribute string       `xml:"AdditionalAttribute,attr" binding:"-"` //Дополнительный реквизит предмета расчета
		ExciseAmount        float64      `xml:"ExciseAmount,attr" binding:"-"`        //Cумма акциза с учетом копеек, включенная в стоимость предмета расчета
	}

	type Positions struct {
		FiscalString []FiscalString `xml:"FiscalString"`
		//Строка с произвольным текстом	Печать текстовой строки.
		TextString string `xml:"TextString,attr" binding:"-"`
		//Печать штрихкода. Осуществляется с автоматическим размером с выравниванием по центру чека. Тип штрихкода может иметь одно из следующих значений: EAN8, EAN13, CODE39, QR. В случае, если модель устройства не поддерживает печать штрихкода вышеуказанных типов, драйвер должен вернуть ошибку.
		Barcode Barcode `xml:"Barcode" binding:"-"`
	}

	type Payments struct {
		Cash              float64 `xml:"Cash,attr" binding:"-"`              //Сумма оплаты наличными денежными средствами	Параметры закрытия чека. Сумма всех видов оплат должна быть больше суммы открытого чека.
		ElectronicPayment float64 `xml:"ElectronicPayment,attr" binding:"-"` //Сумма оплаты безналичными средствами платежа
		PrePayment        float64 `xml:"PrePayment,attr" binding:"-"`        //Сумма зачтенной предоплаты или аванса
		PostPayment       float64 `xml:"PostPayment,attr" binding:"-"`       //Сумма оплаты в кредит (постоплаты)
		Barter            float64 `xml:"Barter,attr" binding:"-"`            //Сумма оплаты встречным предоставлением
	}

	type CheckPackage struct {
		XMLName    xml.Name   `xml:"CheckPackage"`
		Parameters Parameters `xml:"Parameters"`
		Positions  Positions  `xml:"Positions"`
		Payments   Payments   `xml:"Payments"`
	}

	type DocumentOutputParameters struct {
		XMLName xml.Name `xml:"Parameters"`
		//Номер открытой смены/Номер закрытой смены
		ShiftNumber int `xml:"ShiftNumber,attr" binding:"required"`
		//Номер фискального документа
		CheckNumber             int    `xml:"CheckNumber,attr" binding:"required"`
		ShiftClosingCheckNumber int    `xml:"ShiftClosingCheckNumber,attr" binding:"required"` //Номер чека за смену
		AddressSiteInspections  string `xml:"AddressSiteInspections,attr" binding:"required"`  //Адрес сайта проверки
		FiscalSign              string `xml:"FiscalSign,attr" binding:"required"`              //Фискальный признак
		DateTime                string `xml:"DateTime,attr" binding:"required"`                //datetime	//Дата и время формирования документа
	}
	/*
		Признаки способа расчета
		Код	Описание
		1	Предоплата полная
		2	Предоплата частичная
		3	Аванс
		4	Полный расчет
		5	Частичный расчет и кредит
		6	Передача в кредит
		7	Оплата кредита
		Признаки предмета расчета
		Код	Описание
		1	Товар
		2	Подакцизный товар
		3	Работа
		4	Услуга
		5	Ставка азартной игры
		6	Выигрыш азартной игры
		7	Лотерейный билет
		8	Выигрыш лотереи
		9	Предоставление результатов интеллектуальной деятельности
		10	Платеж
		11	Агентское вознаграждение
		12	Выплата
		13	Иной предмет расчета
		14	Имущественное право
		15	Внереализационный доход
		16	Страховые взносы
		17	Торговый сбор
		18	Курортный сбор
		19	Залог
		20	Расход
		21	Взносы на обязательное пенсионное страхование ИП
		22	Взносы на обязательное пенсионное страхование
		23	Взносы на обязательное медицинское страхование ИП
		24	Взносы на обязательное медицинское страхование
		25	Взносы на обязательное социальное страхование
		26	Платеж казино
		Признак агента
		Код	Описание
		0	Банковский платежный агент
		1	Банковский платежный субагент
		2	Платежный агент
		3	Платежный субагент
		4	Поверенный
		5	Комиссионер
		6	Агент
		Код типа маркированной продукции
		Код	Описание
		1	Изделия из меха
		2	Табачная продукция
		3	Обувные товары
		4	Товары легкой промышленности и одежды
		5	Шины и автопокрышки
		6	Молоко и молочная продукция
		7	Фотокамеры и лампы-вспышки
		8	Велосипеды
		9	Кресла-коляски
		10	Духи и туалетная вода

	*/
	var chk = CheckPackage{}
	var out = DocumentOutputParameters{}
	deviceID := c.Param("DeviceID")
	//Формирование чека в только электроном виде. Печать чека не осуществляется.
	//Electronically := c.Param("Electronically")

	out.DateTime = time.Now().Format("2006-01-02") //time.Parse(("2006-01-02", strDate)
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
	if err = c.ShouldBindXML(&chk); err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	errcode, err := kkm.FNGetStatus()
	if err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errcode == 0 {
		fnstate := kkm.FNGetFNState()
		ShiftState := int(fnstate.FNSessionState + 1) //1 - Закрыта 2 - Открыта 3 - Истекла
		if ShiftState != 2 {
			if ShiftState == 3 {
				c.XML(http.StatusBadRequest, gin.H{"error": "Смена истекла, необходимо закрытие"})
			} else {
				c.XML(http.StatusBadRequest, gin.H{"error": "Смена закрыта"})
			}
			return
		}
	} else {
		c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
		return
	}
	//Открыть чек
	//Команда: 8DH. Длина сообщения: 6 байт.
	//Пароль оператора (4 байта) Тип документа (1 байт):
	//«0» – продажа  «1» – покупка  «2» – возврат продажи  «3» – возврат покупки  Код ошибки (1 байт) Порядковый номер оператора (1 байт) 1…30
	tabparam := make([]byte, 5)
	copy(tabparam, admpass)
	//1 - приход денежных средств 		2 - возврат прихода денежных средств
	//3 - расход денежных средств		//4 - возврат расхода денежных средств
	optype := 0
	switch chk.Parameters.PaymentType {
	case 1: //продажа
		tabparam[5] = 0
		optype = 1
	case 2: //возврат продажи
		tabparam[5] = 2
		optype = 2
	case 3: //покупка
		tabparam[5] = 1
		optype = 3
	case 4: //возврат покупки
		tabparam[5] = 3
		optype = 4
	}
	switch chk.Parameters.OperationType {
	case 1: //продажа
		tabparam[5] = 0
		optype = 1
	case 2: //возврат продажи
		tabparam[5] = 2
		optype = 2
	case 3: //покупка
		tabparam[5] = 1
		optype = 3
	case 4: //возврат покупки
		tabparam[5] = 3
		optype = 4
	}
	//open chk
	errcode, _, err = kkm.SendCommand(0x8d, tabparam)
	if errcode > 0 {
		c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
		return
	}
	pass := kkm.GetPass()
	//формируем заголовок
	if len(chk.Parameters.SenderEmail) > 0 {
		kkm.PrintString(pass, chk.Parameters.SenderEmail)
	}
	if len(chk.Parameters.CustomerEmail) > 0 {
		kkm.FNSendTLV(pass, 1008, []byte(encodeWindows1251(chk.Parameters.CustomerEmail)))
	} else {
		if len(chk.Parameters.CustomerPhone) > 0 {
			kkm.FNSendTLV(pass, 1008, []byte(encodeWindows1251(chk.Parameters.CustomerPhone)))
		}
	}
	if len(chk.Parameters.CashierINN) > 0 {
		kkm.FNSendTLV(pass, 1203, []byte(chk.Parameters.CashierINN))
	}
	if len(chk.Parameters.CashierName) > 0 {
		kkm.FNSendTLV(pass, 1021, []byte(encodeWindows1251(chk.Parameters.CashierName)))
	}
	vta := make(map[string]float64)
	for _, fs := range chk.Positions.FiscalString {
		if fs.VATAmount > 0 {
			vta[fs.VATRate] = vta[fs.VATRate] + fs.VATAmount
		} else {
			vta[fs.VATRate] = vta[fs.VATRate] + fs.AmountWithDiscount
		}
		if fs.PaymentMethod == 0 {
			fs.PaymentMethod = 4
		}
		if fs.CalculationSubject == 0 {
			fs.CalculationSubject = 1
		}
		if len(fs.MeasurementUnit) > 0 {
			fs.Name = fs.Name + " " + fs.MeasurementUnit
		}
		errcode, _ = kkm.FNOperation(pass, optype, fs.Quantity, fs.PriceWithDiscount, fs.AmountWithDiscount, fs.VATAmount, fs.VATRate, fs.Department, fs.PaymentMethod, fs.CalculationSubject, fs.Name)
		if errcode > 0 {
			kkm.CancelCheck(pass)
			c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
			return
		}
		//отправим теги
		//AgentData
		//VendorData
		//GoodCodeData        GoodCodeData `xml:"GoodCodeData" binding:"-"`             //Вложенная структура	Данные кода товарной номенклатуры
		//CountryOfOrigin     string       `xml:"CountryOfOrigin,attr" binding:"-"`     //Цифровой код страны происхождения товара в соответствии с Общероссийским классификатором стран мира
		//CustomsDeclaration  string       `xml:"CustomsDeclaration,attr" binding:"-"`  //Регистрационный номер таможенной декларации
		//AdditionalAttribute string       `xml:"AdditionalAttribute,attr" binding:"-"` //Дополнительный реквизит предмета расчета
		//ExciseAmount        float64      `xml:"ExciseAmount,attr" binding:"-"`        //Cумма акциза с учетом копеек, включенная в стоимость предмета расчета
		if fs.CalculationSubject == 2 { //подакцизный товар
			//1207 = 1 byte
			param := make([]byte, 1)
			param[0] = 1 //[]byte(strconv.FormatInt(1,10))
			kkm.FNSendTLVOperation(pass, 1207, param)
			//«признак предмета расчета» (тег 1212 byte), «признак способа расчета» (тег 1214), «наименование предмета расчета» (тег 1030), «количество предмета расчета» (тег 1023) и «цена за единицу предмета расчета» (тег 1079)
			param[0] = byte(fs.CalculationSubject)
			kkm.FNSendTLVOperation(pass, 1212, param)
			param[0] = byte(fs.PaymentMethod)
			kkm.FNSendTLVOperation(pass, 1214, param)
			kkm.FNSendTLVOperation(pass, 1030, []byte(encodeWindows1251(fs.Name)))
			//fs.Quantity 6 byte, fs.PriceWithDiscount 5 byte
			res := strconv.FormatFloat(fs.Quantity, 'f', 6, 64)
			kkm.FNSendTLVOperation(pass, 1023, []byte(res))
			res = strconv.FormatFloat(fs.PriceWithDiscount, 'f', 2, 64)
			kkm.FNSendTLVOperation(pass, 1079, []byte(res))
		}
		//«адрес оператора перевода» (тег 1005), «ИНН оператора перевода» (тег 1016), «наименование оператора перевода» (тег 1026),
		if len(fs.AgentData.AgentOperation) > 0 {
			kkm.FNSendTLVOperation(pass, 1044, []byte(encodeWindows1251(fs.AgentData.AgentOperation)))
		}
		if len(fs.AgentData.AcquirerOperatorINN) > 0 {
			kkm.FNSendTLVOperation(pass, 1016, []byte(encodeWindows1251(fs.AgentData.AcquirerOperatorINN)))
		}
		if len(fs.AgentData.AcquirerOperatorPhone) > 0 {
			kkm.FNSendTLVOperation(pass, 1073, []byte(encodeWindows1251(fs.AgentData.AcquirerOperatorPhone)))
		}
		if len(fs.AgentData.AcquirerOperatorName) > 0 {
			kkm.FNSendTLVOperation(pass, 1026, []byte(encodeWindows1251(fs.AgentData.AcquirerOperatorName)))
		}
		if len(fs.AgentData.AcquirerOperatorAddress) > 0 {
			kkm.FNSendTLVOperation(pass, 1005, []byte(encodeWindows1251(fs.AgentData.AcquirerOperatorAddress)))
		}
		if len(fs.VendorData.VendorINN) > 0 {
			kkm.FNSendTLVOperation(pass, 1226, []byte(fs.VendorData.VendorINN))
		}
		if len(fs.VendorData.VendorName) > 0 {
			kkm.FNSendTLVOperation(pass, 1224, []byte(encodeWindows1251(fs.VendorData.VendorName)))
		}
		if len(fs.MeasurementUnit) > 0 {
			kkm.FNSendTLVOperation(pass, 1197, []byte(encodeWindows1251(fs.MeasurementUnit)))
		}
		//«телефон поставщика» (тег 1171)
		if len(fs.VendorData.VendorPhone) > 0 {
			kkm.FNSendTLVOperation(pass, 1171, []byte(encodeWindows1251(fs.VendorData.VendorName)))
		}
	}

	//доп реквизит пользователя 1084
	if len(chk.Parameters.UserAttribute.Name) > 0 {

	}

	summa := make(map[int]float64)
	summa[1] = chk.Payments.Cash
	summa[2] = chk.Payments.ElectronicPayment
	summa[14] = chk.Payments.PrePayment
	summa[15] = chk.Payments.PostPayment
	summa[16] = chk.Payments.Barter
	_, out.CheckNumber, out.FiscalSign, out.DateTime, errcode, err = kkm.CloseCheck(pass, summa, vta, byte(chk.Parameters.TaxationSystem), 0, "")
	if errcode > 0 {
		kkm.CancelCheck(pass)
		c.XML(http.StatusBadRequest, gin.H{"error": kkm.ParseErrState(errcode)})
		return
	}
	c.XML(http.StatusBadRequest, out)
}
