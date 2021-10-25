# kkm-shtrih
Driver kkm shtrih for linux
Драйвер для ККМ Штрих. Работает по REST api. Один сервер может обслуживать несколько ККМ. При добавлении новой ккм присваивается DeviceID. 
Основные функции:
GET SearchKKM - поиск подключенных ККМ
GET getPorts - поиск COM портов
GET GetServSetting
PUT SetServSetting
POST run/:DeviceID/<command> Выполнит команду ККМ по коду командыю. command код команды ккм (см. документацию штрих). 
GET GetParamKKT/<DeviceID>

		//функции для низкоуровневой работы с чеком
		PUT  SetBusy/<DeviceID> установить ккм в режим занчяо
		PUT Release/<DeviceID> освободить ккм
		POST OpenCheck/<DeviceID> открыть чек
		POST  FNOperation/<DeviceID> выполнить операцию с чеком
		POST PrintString/<DeviceID> печать строки
		POST CancelCheck/<DeviceID> отменить чек
		POST CloseCheck/<DeviceID> закрыть чек
		POST FNSendTagOperation/<DeviceID> отправить tag операции
		POST FNSendTag/<DeviceID>  отправить tag чека на ккм
		POST CutCheck/<DeviceID> отрезать чек

		//1c spec Принимает параметры и возвращает ответ согласно специфиуации 1с. (см сайт 1с)
		POST  GetDataKKT/<DeviceID> получить данные  ккм

		POST OpenShift/<DeviceID> открыть смену
		POST CloseShift/<DeviceID> закрыть смену
		POST ProcessCheck/<DeviceID> операция с чеком
		POST ProcessCorrectionCheck/<DeviceID> чек коррекции
		POST PrintTextDocument/<DeviceID>
		POST CashInOutcome/<DeviceID>
		POST PrintXReport/<DeviceID>
		POST PrintCheckCopy/<DeviceID>
		POST GetCurrentStatus/<DeviceID>
		POST ReportCurrentStatusOfSettlements/:DeviceID
		POST OpenCashDrawer/:DeviceID
		POST GetLineLength/:DeviceID
