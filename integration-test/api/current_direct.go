package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func getExchangeRates(currency string) (*CurrencyResponse, error) {
	resp, err := http.DefaultClient.Get(fmt.Sprintf("https://open.er-api.com/v6/latest/%s", currency))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var currencyResponse CurrencyResponse
	if err := json.NewDecoder(resp.Body).Decode(&currencyResponse); err != nil {
		return nil, err
	}

	return &currencyResponse, nil
}

type CurrencyResponse struct {
	Result             string `json:"result"`
	Provider           string `json:"provider"`
	Documentation      string `json:"documentation"`
	TermsOfUse         string `json:"terms_of_use"`
	TimeLastUpdateUnix int    `json:"time_last_update_unix"`
	TimeLastUpdateUtc  string `json:"time_last_update_utc"`
	TimeNextUpdateUnix int    `json:"time_next_update_unix"`
	TimeNextUpdateUtc  string `json:"time_next_update_utc"`
	TimeEolUnix        int    `json:"time_eol_unix"`
	BaseCode           string `json:"base_code"`
	Rates              struct {
		Usd float64 `json:"USD"`
		Aed float64 `json:"AED"`
		Afn float64 `json:"AFN"`
		All float64 `json:"ALL"`
		Amd float64 `json:"AMD"`
		Ang float64 `json:"ANG"`
		Aoa float64 `json:"AOA"`
		Ars float64 `json:"ARS"`
		Aud float64 `json:"AUD"`
		Awg float64 `json:"AWG"`
		Azn float64 `json:"AZN"`
		Bam float64 `json:"BAM"`
		Bbd float64 `json:"BBD"`
		Bdt float64 `json:"BDT"`
		Bgn float64 `json:"BGN"`
		Bhd float64 `json:"BHD"`
		Bif float64 `json:"BIF"`
		Bmd float64 `json:"BMD"`
		Bnd float64 `json:"BND"`
		Bob float64 `json:"BOB"`
		Brl float64 `json:"BRL"`
		Bsd float64 `json:"BSD"`
		Btn float64 `json:"BTN"`
		Bwp float64 `json:"BWP"`
		Byn float64 `json:"BYN"`
		Bzd float64 `json:"BZD"`
		Cad float64 `json:"CAD"`
		Cdf float64 `json:"CDF"`
		Chf float64 `json:"CHF"`
		Clp float64 `json:"CLP"`
		Cny float64 `json:"CNY"`
		Cop float64 `json:"COP"`
		Crc float64 `json:"CRC"`
		Cup float64 `json:"CUP"`
		Cve float64 `json:"CVE"`
		Czk float64 `json:"CZK"`
		Djf float64 `json:"DJF"`
		Dkk float64 `json:"DKK"`
		Dop float64 `json:"DOP"`
		Dzd float64 `json:"DZD"`
		Egp float64 `json:"EGP"`
		Ern float64 `json:"ERN"`
		Etb float64 `json:"ETB"`
		Eur float64 `json:"EUR"`
		Fjd float64 `json:"FJD"`
		Fkp float64 `json:"FKP"`
		Fok float64 `json:"FOK"`
		Gbp float64 `json:"GBP"`
		Gel float64 `json:"GEL"`
		Ggp float64 `json:"GGP"`
		Ghs float64 `json:"GHS"`
		Gip float64 `json:"GIP"`
		Gmd float64 `json:"GMD"`
		Gnf float64 `json:"GNF"`
		Gtq float64 `json:"GTQ"`
		Gyd float64 `json:"GYD"`
		Hkd float64 `json:"HKD"`
		Hnl float64 `json:"HNL"`
		Hrk float64 `json:"HRK"`
		Htg float64 `json:"HTG"`
		Huf float64 `json:"HUF"`
		Idr float64 `json:"IDR"`
		Ils float64 `json:"ILS"`
		Imp float64 `json:"IMP"`
		Inr float64 `json:"INR"`
		Iqd float64 `json:"IQD"`
		Irr float64 `json:"IRR"`
		Isk float64 `json:"ISK"`
		Jep float64 `json:"JEP"`
		Jmd float64 `json:"JMD"`
		Jod float64 `json:"JOD"`
		Jpy float64 `json:"JPY"`
		Kes float64 `json:"KES"`
		Kgs float64 `json:"KGS"`
		Khr float64 `json:"KHR"`
		Kid float64 `json:"KID"`
		Kmf float64 `json:"KMF"`
		Krw float64 `json:"KRW"`
		Kwd float64 `json:"KWD"`
		Kyd float64 `json:"KYD"`
		Kzt float64 `json:"KZT"`
		Lak float64 `json:"LAK"`
		Lbp float64 `json:"LBP"`
		Lkr float64 `json:"LKR"`
		Lrd float64 `json:"LRD"`
		Lsl float64 `json:"LSL"`
		Lyd float64 `json:"LYD"`
		Mad float64 `json:"MAD"`
		Mdl float64 `json:"MDL"`
		Mga float64 `json:"MGA"`
		Mkd float64 `json:"MKD"`
		Mmk float64 `json:"MMK"`
		Mnt float64 `json:"MNT"`
		Mop float64 `json:"MOP"`
		Mru float64 `json:"MRU"`
		Mur float64 `json:"MUR"`
		Mvr float64 `json:"MVR"`
		Mwk float64 `json:"MWK"`
		Mxn float64 `json:"MXN"`
		Myr float64 `json:"MYR"`
		Mzn float64 `json:"MZN"`
		Nad float64 `json:"NAD"`
		Ngn float64 `json:"NGN"`
		Nio float64 `json:"NIO"`
		Nok float64 `json:"NOK"`
		Npr float64 `json:"NPR"`
		Nzd float64 `json:"NZD"`
		Omr float64 `json:"OMR"`
		Pab float64 `json:"PAB"`
		Pen float64 `json:"PEN"`
		Pgk float64 `json:"PGK"`
		Php float64 `json:"PHP"`
		Pkr float64 `json:"PKR"`
		Pln float64 `json:"PLN"`
		Pyg float64 `json:"PYG"`
		Qar float64 `json:"QAR"`
		Ron float64 `json:"RON"`
		Rsd float64 `json:"RSD"`
		Rub float64 `json:"RUB"`
		Rwf float64 `json:"RWF"`
		Sar float64 `json:"SAR"`
		Sbd float64 `json:"SBD"`
		Scr float64 `json:"SCR"`
		Sdg float64 `json:"SDG"`
		Sek float64 `json:"SEK"`
		Sgd float64 `json:"SGD"`
		Shp float64 `json:"SHP"`
		Sle float64 `json:"SLE"`
		Sll float64 `json:"SLL"`
		Sos float64 `json:"SOS"`
		Srd float64 `json:"SRD"`
		Ssp float64 `json:"SSP"`
		Stn float64 `json:"STN"`
		Syp float64 `json:"SYP"`
		Szl float64 `json:"SZL"`
		Thb float64 `json:"THB"`
		Tjs float64 `json:"TJS"`
		Tmt float64 `json:"TMT"`
		Tnd float64 `json:"TND"`
		Top float64 `json:"TOP"`
		Try float64 `json:"TRY"`
		Ttd float64 `json:"TTD"`
		Tvd float64 `json:"TVD"`
		Twd float64 `json:"TWD"`
		Tzs float64 `json:"TZS"`
		Uah float64 `json:"UAH"`
		Ugx float64 `json:"UGX"`
		Uyu float64 `json:"UYU"`
		Uzs float64 `json:"UZS"`
		Ves float64 `json:"VES"`
		Vnd float64 `json:"VND"`
		Vuv float64 `json:"VUV"`
		Wst float64 `json:"WST"`
		Xaf float64 `json:"XAF"`
		Xcd float64 `json:"XCD"`
		Xcg float64 `json:"XCG"`
		Xdr float64 `json:"XDR"`
		Xof float64 `json:"XOF"`
		Xpf float64 `json:"XPF"`
		Yer float64 `json:"YER"`
		Zar float64 `json:"ZAR"`
		Zmw float64 `json:"ZMW"`
		Zwl float64 `json:"ZWL"`
	} `json:"rates"`
}
