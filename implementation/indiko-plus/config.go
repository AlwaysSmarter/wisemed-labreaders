package main

func otherConfig(tplData map[string]string) string {
	return `
 <div class="ace-col-12" style="display: flex">
	<div class="ace-col-5 optional-form ` + tplData["EQUIPMENT_FORM_HIDDEN"] + `" ` + tplData["EQUIPMENT_FORM_OPTIONAL"] + ` style="background-color: #ffedd9;">
    	<h3>Equipment WS configuration</h3>
		<div class="config">
			<div class="label">Protocol type</div>
			<div>
				<select class="ace-col-12" name="othercfg_sysmexxn550_comm_type" val="` + tplData["othercfg_sysmexxn550_comm_type"] + `">
					<option value="1">ASTM 1381-02 ASTM 1894_97</option>
				</select>
			</div>
		</div>
	</div>
</div>`
}

func defaultKTData() []map[string]string {
	kt := []map[string]string{
		map[string]string{"code": "ALP", "name": "ALP", "um": "", "restype": "1"},
		map[string]string{"code": "ALT", "name": "ALT", "um": "", "restype": "1"},
		map[string]string{"code": "AMY", "name": "AMY", "um": "", "restype": "1"},
		map[string]string{"code": "APO A1", "name": "APO A1", "um": "", "restype": "1", "inactive": "1"},
		map[string]string{"code": "APO B", "name": "APO B", "um": "", "restype": "1", "inactive": "1"},
		map[string]string{"code": "ASO", "name": "ASO", "um": "", "restype": "1", "inactive": "1"},
		map[string]string{"code": "AST", "name": "AST", "um": "", "restype": "1"},
		map[string]string{"code": "CA", "name": "Calcium", "um": "", "restype": "1"},
		map[string]string{"code": "CHOL", "name": "CHOL", "um": "", "restype": "1"},
		map[string]string{"code": "CK", "name": "CK2", "um": "", "restype": "1"},
		map[string]string{"code": "CREA", "name": "Crea Comp", "um": "", "restype": "1"},
		map[string]string{"code": "DBIL", "name": "Direct Bilirubin", "um": "", "restype": "1"},
		map[string]string{"code": "GGT", "name": "GGT", "um": "", "restype": "1"},
		map[string]string{"code": "GLUC", "name": "Glucose", "um": "", "restype": "1"},
		map[string]string{"code": "HDL", "name": "HDL", "um": "", "restype": "1"},
		map[string]string{"code": "HDLC", "name": "HDLC", "um": "", "restype": "1"},
		map[string]string{"code": "IRON", "name": "IRON", "um": "", "restype": "1"},
		map[string]string{"code": "LDH", "name": "LDH", "um": "", "restype": "1"},
		map[string]string{"code": "MG", "name": "Magensium", "um": "", "restype": "1"},
		map[string]string{"code": "TBIL", "name": "Total Bilirubin", "um": "", "restype": "1"},
		map[string]string{"code": "TPROT", "name": "Total Protein", "um": "", "restype": "1"},
		map[string]string{"code": "TRIG", "name": "Triglicerides", "um": "", "restype": "1"},
		map[string]string{"code": "URAC", "name": "Uric Acid", "um": "", "restype": "1"},
		map[string]string{"code": "UREA", "name": "Urea", "um": "", "restype": "1"},
	}

	return kt
}
