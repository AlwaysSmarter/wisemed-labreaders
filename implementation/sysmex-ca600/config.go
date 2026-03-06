package main

func otherConfig(tplData map[string]string) string {
	return `
 <div class="ace-col-12" style="display: flex">
	<div class="ace-col-5 optional-form ` + tplData["EQUIPMENT_FORM_HIDDEN"] + `" ` + tplData["EQUIPMENT_FORM_OPTIONAL"] + ` style="background-color: #ffedd9;">
    	<h3>Equipment WS configuration</h3>
		<div class="config">
			<div class="label">Comm type</div>
			<div>
				<select class="ace-col-12" name="othercfg_sysmexca600_comm_type" val="` + tplData["othercfg_sysmexca600_comm_type"] + `">
					<option value="">Text code</option>
					<option value="1">Numeric code</option>
				</select>
			</div>
		</div>
	</div>
</div>`
}

func defaultKTData() []map[string]string {
	kt := []map[string]string{
		map[string]string{"code": "041", "name": "PT T", "um": "sec"},
		map[string]string{"code": "042", "name": "PT%", "um": " %"},
		map[string]string{"code": "043", "name": "PT R.", "um": "-"},
		map[string]string{"code": "044", "name": "PT INR", "um": "-"},
		map[string]string{"code": "045", "name": "dFbg", "um": "mg/dL"},
		map[string]string{"code": "051", "name": "FS", "um": "sec"},
		map[string]string{"code": "061", "name": "Fbg", "um": "sec"},
		map[string]string{"code": "062", "name": "Fbg C.", "um": "mg/dL"},
		map[string]string{"code": "121", "name": "II", "um": "sec"},
		map[string]string{"code": "122", "name": "II%", "um": "%"},
		map[string]string{"code": "151", "name": "V", "um": "sec"},
		map[string]string{"code": "152", "name": "V%", "um": "%"},
		map[string]string{"code": "171", "name": "VII", "um": "sec"},
		map[string]string{"code": "172", "name": "VII%", "um": "%"},
		map[string]string{"code": "181", "name": "VIII", "um": "sec"},
		map[string]string{"code": "182", "name": "VIII%", "um": "%"},
		map[string]string{"code": "191", "name": "IX", "um": "sec"},
		map[string]string{"code": "192", "name": "IX%", "um": "%"},
		map[string]string{"code": "201", "name": "X", "um": "sec"},
		map[string]string{"code": "202", "name": "X%", "um": "%"},
		map[string]string{"code": "211", "name": "XI", "um": "sec"},
		map[string]string{"code": "212", "name": "XI%", "um": "%"},
		map[string]string{"code": "221", "name": "XII", "um": "sec"},
		map[string]string{"code": "312", "name": "APL%", "um": "%"},
		map[string]string{"code": "321", "name": "Plg", "um": "dOD"},
		map[string]string{"code": "322", "name": "Plg%", "um": "%"},
		map[string]string{"code": "331", "name": "BCPC", "um": "dOD"},
		map[string]string{"code": "332", "name": "BCPC%", "um": "%"},
		map[string]string{"code": "341", "name": "Hep", "um": "dOD"},
		map[string]string{"code": "342", "name": "Hep", "um": "U/mL"},
		map[string]string{"code": "501", "name": "#NAME?", "um": "sec"},
		map[string]string{"code": "502", "name": "Fbg C.", "um": "mg/dL"},
		map[string]string{"code": "511", "name": "TT", "um": "sec"},
		map[string]string{"code": "521", "name": "#NAME?", "um": "sec"},
		map[string]string{"code": "522", "name": "Fbg C.", "um": "mg/dL"},
		map[string]string{"code": "611", "name": "DDPl", "um": "dOD"},
	}

	return kt
}
