package hl7_handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lenaten/hl7"
	"strconv"
	"strings"
	"time"
	"wisemed-labreaders/config"
	"wisemed-labreaders/protocols/hl7/hl7_segments"
	"wisemed-labreaders/sqlitewrapper"
	"wisemed-labreaders/wisemed"
)

// Handle the MeasurementResults (OUL^R22^OUL_R22) message came from the analyzers
// The message from the analyzer consists of 2 segments:
//
//			MSH
//		 [PID]
//			SPM
//		 SAC
//		 [INV]
//	  {
//	   OBR
//	   ORC
//	   TQ1
//
//	  }
//
// Example:
// \vMSH|^~\&|cobas pure||Host||20221104154049+0900||OUL^R22^OUL_R22|2128|P|2.5.1|||NE|AL||UNICODE UTF-8|||LAB-29^IHE\rPID|||||^^^^^^U|||U\rSPM|1|498151&BARCODE||SERPLAS^^99ROC|||||||P^^HL70369|||~~~~||||||||||PSCO^^99ROC\rSAC|||498151^BARCODE|||||||50001|1||||||||||||||||||^1^:^1\rOBR|1|""||20130^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20130^20130^99ROC^^^IHELAW|1|18.0|U/L^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|2|CE|20130^20130^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rTCD|20130^^99ROC|^1^:^1\rINV|2013001|OK^^HL70383~CURRENT^^99ROC|R1|1699|1|26||||||20231130||||661052\rINV|2012001|OK^^HL70383~CURRENT^^99ROC|SPR|5554|1|21||||||20231130||||654893\rINV|2013001|OK^^HL70383~CURRENT^^99ROC|R3|1699|1|26||||||20231130||||661052\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134127|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|29|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|18~1|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|5.00 - 700|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBR|2|""||20230^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20230^20230^99ROC^^^IHELAW|1|16.5|U/L^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|2|CE|20230^20230^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rTCD|20230^^99ROC|^1^:^1\rINV|2022001|OK^^HL70383~CURRENT^^99ROC|R1|5196|1|27||||||20230930||||644916\rINV|2012001|OK^^HL70383~CURRENT^^99ROC|SPR|5554|1|21||||||20231130||||654893\rINV|2022001|OK^^HL70383~CURRENT^^99ROC|R3|5196|1|27||||||20230930||||644916\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005132431|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|31|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|19~2|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|5.00 - 700|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBR|3|""||20140^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20140^20140^99ROC^^^IHELAW|1|8.82|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|2|CE|20140^20140^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rTCD|20140^^99ROC|^1^:^1\rINV|2034001|OK^^HL70383~CURRENT^^99ROC|R1|2571|1|22||||||20230930||||645917\rINV|2034001|OK^^HL70383~CURRENT^^99ROC|R3|2571|1|22||||||20230930||||645917\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005132439|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|32|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20~3|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|0.802 - 20.1|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBR|4|""||20411^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20411^20411^99ROC^^^IHELAW|1|157|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|2|CE|20411^20411^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rTCD|20411^^99ROC|^1^:^1\rINV|2041001|OK^^HL70383~CURRENT^^99ROC|R1|1008|1|3||||||20230228||||640736\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134135|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|33|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|21~4|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.87 - 800|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBR|5|""||20470^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20470^20470^99ROC^^^IHELAW|1|0.992|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|2|CE|20470^20470^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rTCD|20470^^99ROC|^1^:^1\rINV|2047001|OK^^HL70383~CURRENT^^99ROC|R1|444|1|41||||||20231231||||627775\rINV|2047001|OK^^HL70383~CURRENT^^99ROC|R3|444|1|41||||||20231231||||627775\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134143|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|34|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|22~5|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|0.170 - 24.9|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-113000 - 113000|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-113000 - 113000|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBR|6|""||20600^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20600^20600^99ROC^^^IHELAW|1|12.7|U/L^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|2|CE|20600^20600^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rTCD|20600^^99ROC|^1^:^1\rINV|2060001|OK^^HL70383~CURRENT^^99ROC|R1|4971|1|8||||||20230531||||653840\rINV|2060001|OK^^HL70383~CURRENT^^99ROC|R3|4971|1|8||||||20230531||||653840\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134151|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|37|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|25~8|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.00 - 1200|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBR|7|""||20630^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20630^20630^99ROC^^^IHELAW|1|92.3|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|2|CE|20630^20630^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rTCD|20630^^99ROC|^1^:^1\rINV|2063001|OK^^HL70383~CURRENT^^99ROC|R1|1016|1|4||||||20230731||||629487\rINV|2063001|OK^^HL70383~CURRENT^^99ROC|R3|1016|1|4||||||20230731||||629487\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134159|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|38|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|26~9|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1.98 - 750|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBR|8|""||20710^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20710^20710^99ROC^^^IHELAW|1|41.1|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|2|CE|20710^20710^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rTCD|20710^^99ROC|^1^:^1\rINV|2071001|OK^^HL70383~CURRENT^^99ROC|R1|7837|1|5||||||20231231||||626113\rINV|2071001|OK^^HL70383~CURRENT^^99ROC|R3|7837|1|5||||||20231231||||626113\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134207|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|41|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|27~10|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.09 - 150|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBR|9|""||20820^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20820^20820^99ROC^^^IHELAW|1|106|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|2|CE|20820^20820^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rTCD|20820^^99ROC|^1^:^1\rINV|2082001|OK^^HL70383~CURRENT^^99ROC|R1|846|1|18||||||20231031||||590647\rINV|2082001|OK^^HL70383~CURRENT^^99ROC|R3|846|1|18||||||20231031||||590647\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134215|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|43|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|29~12|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20\r221005135234||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.87 - 549|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBR|10|""||21130^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|21130^21130^99ROC^^^IHELAW|1|76.9|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|2|CE|21130^21130^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rTCD|21130^^99ROC|^1^:^1\rINV|2113001|OK^^HL70383~CURRENT^^99ROC|R1|4123|1|19||||||20230630||||643364\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134223|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|46|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|32~15|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|8.85 - 885|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBR|11|""||21170^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|21170^21170^99ROC^^^IHELAW|1|5.15|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|2|CE|21170^21170^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rTCD|21170^^99ROC|^1^:^1\rINV|2117001|OK^^HL70383~CURRENT^^99ROC|R1|298|1|7||||||20230430||||627564\rINV|2117001|OK^^HL70383~CURRENT^^99ROC|R3|298|1|7||||||20230430||||627564\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134231|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|47|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|33~16|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|0.200 - 25.0|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBR|12|""||21191^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|21191^21191^99ROC^^^IHELAW|1|40.8|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|2|CE|21191^21191^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rTCD|21191^^99ROC|^1^:^1\rINV|2119001|OK^^HL70383~CURRENT^^99ROC|R1|17429|1|24||||||20230630||||663424\rINV|2119001|OK^^HL70383~CURRENT^^99ROC|R3|17429|1|24||||||20230630||||663424\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134239|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|50|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|46~44|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.00 - 240|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\r\x1c\r
// Host should respond with HL7MessageTypes.HL7MsgType_MeasurementResultsMessageACK (ACK^U01^ACK)
func HandleHL7MsgType_MeasurementResults(msg *hl7.Message, mbc config.WSMessageBroadcaster, ac config.AnalyzerConnection) ([]string, error) {
	parseErrLocation := hl7_segments.HL7ErrorWithLocation{}
	msh := hl7_segments.HL7MSH{}
	pid := hl7_segments.HL7PID{}
	spm := hl7_segments.HL7SPM{}
	sac := hl7_segments.HL7SAC{}
	invQC := hl7_segments.HL7INV_ReagentsInQCResult{}
	var fileId int
	//var obrSegments []*hl7.Segment
	//var orcSegments []*hl7.Segment
	//var tq1Segments []*hl7.Segment

	allSeg, err := HL7MapAllSegments(msg.Segments)
	printMap(5, allSeg)
	mshSeg := returnSegmentFromMap("MSH", allSeg)
	err = msh.UnmarshallFromSeg(mshSeg, mshSeg)
	if err != nil {
		parseErrLocation.Location = "MSH^0^^^^"
		parseErrLocation.ErrorText = "An error has occured on unmarshaling MSH segment"
		ackMsg, err := buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_ACKToResultUpload, "AE", &parseErrLocation)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
		return []string{ackMsg}, nil
	}

	pidSeg := returnSegmentFromMap("PID", allSeg)
	err = pid.UnmarshallFromSeg(mshSeg, pidSeg)

	spmSeg := returnSegmentFromMap("SPM", allSeg)
	err = spm.UnmarshallFromSeg(mshSeg, spmSeg)
	if err != nil {
		parseErrLocation.Location = "SPM^0^^^^"
		parseErrLocation.ErrorText = "An error has occured on unmarshaling SPM segment"
		ackMsg, err := buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_ACKToResultUpload, "AE", &parseErrLocation)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
		return []string{ackMsg}, nil
	}

	sacSeg := returnSegmentFromMap("SAC", allSeg)
	err = sac.UnmarshallFromSeg(mshSeg, sacSeg)
	if err != nil {
		parseErrLocation.Location = "SAC^0^^^^"
		parseErrLocation.ErrorText = "An error has occured on unmarshaling SAC segment"
		ackMsg, err := buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_ACKToResultUpload, "AE", &parseErrLocation)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
		return []string{ackMsg}, nil
	}

	//if QC then I might have an INV segment here

	invQCSeg := returnSegmentFromMap("INV", allSeg)
	invQC.UnmarshallFromSeg(mshSeg, invQCSeg)

	//check if I have results data - OBR segment
	if _, ok := allSeg["OBR"]; !ok {
		parseErrLocation.Location = "OBR^0^^^^"
		parseErrLocation.ErrorText = "An error has occured on unmarshaling OBR segment"
		ackMsg, err := buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_ACKToResultUpload, "AE", &parseErrLocation)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
		return []string{ackMsg}, nil
	} else {
		//If I have only one result, then OBR segment is not properly formed - meaning that is not including the rest of the fields there and they are all included in allSeg
		//That is why i have to reformat allSeg a little bit
		_, ok := allSeg["OBR"].([]HL7SubSegments)
		if !ok {
			//here I do the magic
			myObrSeg := HL7SubSegments{MainSegment: allSeg["OBR"].(hl7.Segment)}
			//now adding it's subsegments
			myObrSeg.SubSegments = map[string]interface{}{}
			myObrSeg.SubSegments["OBR"] = allSeg["OBR"]
			if orcSeg, ok := allSeg["ORC"]; ok {
				myObrSeg.SubSegments["ORC"] = orcSeg
				delete(allSeg, "ORC")
			}
			if tq1Seg, ok := allSeg["TQ1"]; ok {
				myObrSeg.SubSegments["TQ1"] = tq1Seg
				delete(allSeg, "TQ1")
			}
			if obxSeg, ok := allSeg["OBX"]; ok {
				myObrSeg.SubSegments["OBX"] = obxSeg
				delete(allSeg, "OBX")
			}

			allSeg["OBR"] = []HL7SubSegments{myObrSeg}
		}
	}

	//first load the patient
	haveQC := false
	fileId = 0
	controlCode := ""
	lotNo := ""

	switch spm.SampleType {
	case "BARCODE":
		haveQC = false
		fileId, err = strconv.Atoi(spm.SampleSeqId)
		fmt.Printf("\nI have a new BARCODE fileID: %d\n", fileId)
		if err != nil {
			fileId = 0
		}
		break
	case "CONTROL":
		haveQC = true
		controlCode = spm.SampleSeqId
		fmt.Printf("\nI have a new CONTROL code: %d\n", controlCode)
		break
	case "SEQUENCE":
		haveQC = false
		fmt.Println("I have a new SEQUENCE")
		break
	}

	fmt.Printf("\nParsing Specimen Role ID: %s Sample Type: %s\n", spm.SpecimenRoleId, sac.SampleType)
	switch spm.SpecimenRoleId {
	case "P":
		if haveQC {
			//maybe error?
		}
		haveQC = false
		break
	case "Q":
		if !haveQC {
			//maybe error?
		}
		haveQC = true
		break
	default:
		//
	}

	switch sac.SampleType {
	case "BARCODE":
		if haveQC {
			//maybe error?
		}
		if fileId <= 0 {
			fileId, err = strconv.Atoi(sac.SampleSeqId)
			if err != nil {
				fileId = 0
			}
		}
		haveQC = false
		break
	case "CONTROL":
		if !haveQC {
			//maybe error?
		}
		haveQC = true
		lotNo = sac.RackId
		break
	case "CALIBRATOR":
		break
	case "SEQUENCE":
		break
	}

	if fileId <= 0 && pid.PatientId != "" {
		fileId, err = strconv.Atoi(pid.PatientId)
		fmt.Printf("\nparsing fileID from patient ID: %d\n", fileId)
		if err != nil {
			fileId = 0
		}
	}

	if !haveQC {
		go func() {
			err := SaveHL7OrderResults(mbc, ac, fileId, allSeg["OBR"], mshSeg)
			if err != nil {
				mbc.BroadcastWSMessage(ac, "runtimererr", err.Error())
			}
		}()
	} else {
		go func() {
			err := SaveHL7QCResults(mbc, ac, controlCode, lotNo, allSeg["OBR"], mshSeg)
			if err != nil {
				mbc.BroadcastWSMessage(ac, "runtimererr", err.Error())
			}
		}()
	}

	ackMsg, err := buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_ACKToResultUpload, "AA", nil)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return []string{ackMsg}, nil
}

func SaveHL7QCResults(mbc config.WSMessageBroadcaster, ac config.AnalyzerConnection, controlCode string, lotNo string, obrData interface{}, mshSeg hl7.Segment) error {
	return nil
}

func SaveHL7OrderResults(mbc config.WSMessageBroadcaster, ac config.AnalyzerConnection, fileId int, obrData interface{}, mshSeg hl7.Segment) error {
	fmt.Printf("\nSaveHL7OrderResults")
	nowt := time.Now()
	obrSegment := hl7_segments.HL7OBR{}
	obxSegment := hl7_segments.HL7OBX{}
	//first load order from DB
	readerOrderSQLITE, _, err := wisemed.LoadFileFromWMAsObj(nowt.Format("2006-01-02"), -1, -1, -1, fileId)
	if err != nil {
		return err
	}
	readerOrderSQLITE.FormatDatesForDB()
	someDataChanged := false
	//fmt.Println(orderData)
	var obrArr []HL7SubSegments
	obrArr, ok := obrData.([]HL7SubSegments)
	if !ok {
		return errors.New("Given OBR segment nor properly formed #1")
	}

	/**here we have an OBR with format array

	"OBR" => [
		MainSegment -> same "OBR" as the key as hl7.Segment
		Subsegments -> [
			"ORC" hl7.Segment
			"TQ1" hl7.Segment
			"OBX" => [
				MainSegment -> same "OBR" as the key as hl7.Segment
				Subsegments -> [
					"ORC" hl7.Segment
					"TQ1" hl7.Segment
					"TCD" hl7.Segment
					{"INV"} hl7.Segment
					"OBX" => [
						MainSegment => same "OBX" as the key as hl7.Segment
						SunSegments => [
							"OBX" => [
								MainSegment => same "OBX" as the key as hl7.Segment
								SunSegments => []
							]
						]
					]
				]
			]
	]

	*/

	for _, obj := range obrArr {
		tmpFld, err := obj.MainSegment.Field(0)
		if err != nil {
			fmt.Printf("\nWARNING: cannot get the type of the main OBR segment: %q\n, error: %q\n", string(obj.MainSegment.Value), err)
			continue
		}

		if string(tmpFld.Value) != "OBR" {
			fmt.Printf("\nWARNING: skipping segment: %s\n", string(tmpFld.Value))
			continue
		}
		err = obrSegment.UnmarshallFromSeg(mshSeg, obj.MainSegment)
		if err != nil {
			fmt.Printf("\nERROR: cannot unmarshal OBR segment: %q\n", string(obj.MainSegment.Value))
			continue
		}

		fmt.Printf("\n\n\nANOTHER OBR: %q OBR TEST CODE: %s \n", string(obj.MainSegment.Value), obrSegment.USIId)

		obxSeg, ok := obj.SubSegments["OBX"]
		if !ok {
			continue
		}
		//If I have only ome:
		obxSegHL7, ok := obxSeg.(hl7.Segment)
		if ok {
			fmt.Printf("\nI HAVE ONE OBX: %q", obxSegHL7)
		} else {
			//check if is array of HL7SubSegments
			fmt.Printf("\nI DON'T HAVE ONE OBX: %q", obxSegHL7)
			obxAarr, ok := obxSeg.([]HL7SubSegments)
			if !ok {
				fmt.Printf("\nI DON'T HAVE AN OBX ARR TOO: %q\n", obxSeg)
				continue
			}

			foundObxResult := false
			for _, obxSubSeg := range obxAarr {
				err = obxSegment.UnmarshallFromSeg(mshSeg, obxSubSeg.MainSegment)
				if err != nil {
					fmt.Printf("\nERROR: cannot unmarshal OBX segment: %q\n", string(obj.MainSegment.Value))
					continue
				}
				if obxSegment.ValueType == "NM" {
					if obxSegment.OIAlternateId == "" {
						foundTest := ""
						for tstIdx, tst := range readerOrderSQLITE.Tests {
							if tst.Code == obrSegment.USIId {
								readerOrderSQLITE.Tests[tstIdx].Raw = obxSegment.ObservationResult
								readerOrderSQLITE.Tests[tstIdx].Result = obxSegment.ObservationResult
								foundTest = obxSegment.ObservationResult
								someDataChanged = true
								break
							}
						}

						if foundTest == "" {
							test := sqlitewrapper.SQLTest{}
							test.Name = "[an]" + obrSegment.USIId
							test.Code = obrSegment.USIId
							test.OrderId = readerOrderSQLITE.Id
							test.Raw = obxSegment.ObservationResult
							someDataChanged = true
							readerOrderSQLITE.Tests = append(readerOrderSQLITE.Tests, test)
							fmt.Printf("Not found - adding as analyzer test", true, "logamsg")
						}

						foundObxResult = true
					}
				}

				if foundObxResult {
					break
				}
			}
		}
	}

	if someDataChanged {
		err = readerOrderSQLITE.SaveFromCommToDB()
		if err != nil {
			return err
		}

		changedResult := map[string]string{
			"file_id": readerOrderSQLITE.FileId,
			"round":   strconv.Itoa(readerOrderSQLITE.RoundNo),
			"rack":    strconv.Itoa(readerOrderSQLITE.RackNo),
			"pos":     strconv.Itoa(readerOrderSQLITE.PositionNo),
		}
		newResData, err := json.Marshal(changedResult)
		if err == nil {
			mbc.BroadcastWSMessage(ac, "newresult", string(newResData))
		} else {
			mbc.BroadcastWSMessage(ac, "newresult", "")
		}
	}
	//\vMSH|^~\&|cobas pure||Host||20221104154049+0900||OUL^R22^OUL_R22|2128|P|2.5.1|||NE|AL||UNICODE UTF-8|||LAB-29^IHE\rPID|||||^^^^^^U|||U\rSPM|1|498151&BARCODE||SERPLAS^^99ROC|||||||P^^HL70369|||~~~~||||||||||PSCO^^99ROC\rSAC|||498151^BARCODE|||||||50001|1||||||||||||||||||^1^:^1\rOBR|1|""||20130^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20130^20130^99ROC^^^IHELAW|1|18.0|U/L^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|2|CE|20130^20130^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rTCD|20130^^99ROC|^1^:^1\rINV|2013001|OK^^HL70383~CURRENT^^99ROC|R1|1699|1|26||||||20231130||||661052\rINV|2012001|OK^^HL70383~CURRENT^^99ROC|SPR|5554|1|21||||||20231130||||654893\rINV|2013001|OK^^HL70383~CURRENT^^99ROC|R3|1699|1|26||||||20231130||||661052\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134127|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|29|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|18~1|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|5.00 - 700|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135146||||||||||RSLT\rOBR|2|""||20230^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20230^20230^99ROC^^^IHELAW|1|16.5|U/L^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|2|CE|20230^20230^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rTCD|20230^^99ROC|^1^:^1\rINV|2022001|OK^^HL70383~CURRENT^^99ROC|R1|5196|1|27||||||20230930||||644916\rINV|2012001|OK^^HL70383~CURRENT^^99ROC|SPR|5554|1|21||||||20231130||||654893\rINV|2022001|OK^^HL70383~CURRENT^^99ROC|R3|5196|1|27||||||20230930||||644916\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005132431|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|31|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|19~2|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|5.00 - 700|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133450||||||||||RSLT\rOBR|3|""||20140^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20140^20140^99ROC^^^IHELAW|1|8.82|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|2|CE|20140^20140^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rTCD|20140^^99ROC|^1^:^1\rINV|2034001|OK^^HL70383~CURRENT^^99ROC|R1|2571|1|22||||||20230930||||645917\rINV|2034001|OK^^HL70383~CURRENT^^99ROC|R3|2571|1|22||||||20230930||||645917\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005132439|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|32|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20~3|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|0.802 - 20.1|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005133458||||||||||RSLT\rOBR|4|""||20411^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20411^20411^99ROC^^^IHELAW|1|157|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|2|CE|20411^20411^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rTCD|20411^^99ROC|^1^:^1\rINV|2041001|OK^^HL70383~CURRENT^^99ROC|R1|1008|1|3||||||20230228||||640736\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134135|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|33|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|21~4|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.87 - 800|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135154||||||||||RSLT\rOBR|5|""||20470^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20470^20470^99ROC^^^IHELAW|1|0.992|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|2|CE|20470^20470^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rTCD|20470^^99ROC|^1^:^1\rINV|2047001|OK^^HL70383~CURRENT^^99ROC|R1|444|1|41||||||20231231||||627775\rINV|2047001|OK^^HL70383~CURRENT^^99ROC|R3|444|1|41||||||20231231||||627775\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134143|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|34|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|22~5|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|0.170 - 24.9|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-113000 - 113000|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-113000 - 113000|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135202||||||||||RSLT\rOBR|6|""||20600^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20600^20600^99ROC^^^IHELAW|1|12.7|U/L^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|2|CE|20600^20600^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rTCD|20600^^99ROC|^1^:^1\rINV|2060001|OK^^HL70383~CURRENT^^99ROC|R1|4971|1|8||||||20230531||||653840\rINV|2060001|OK^^HL70383~CURRENT^^99ROC|R3|4971|1|8||||||20230531||||653840\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134151|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|37|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|25~8|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.00 - 1200|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135210||||||||||RSLT\rOBR|7|""||20630^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20630^20630^99ROC^^^IHELAW|1|92.3|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|2|CE|20630^20630^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rTCD|20630^^99ROC|^1^:^1\rINV|2063001|OK^^HL70383~CURRENT^^99ROC|R1|1016|1|4||||||20230731||||629487\rINV|2063001|OK^^HL70383~CURRENT^^99ROC|R3|1016|1|4||||||20230731||||629487\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134159|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|38|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|26~9|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1.98 - 750|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135218||||||||||RSLT\rOBR|8|""||20710^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20710^20710^99ROC^^^IHELAW|1|41.1|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|2|CE|20710^20710^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rTCD|20710^^99ROC|^1^:^1\rINV|2071001|OK^^HL70383~CURRENT^^99ROC|R1|7837|1|5||||||20231231||||626113\rINV|2071001|OK^^HL70383~CURRENT^^99ROC|R3|7837|1|5||||||20231231||||626113\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134207|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|41|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|27~10|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.09 - 150|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135226||||||||||RSLT\rOBR|9|""||20820^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|20820^20820^99ROC^^^IHELAW|1|106|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|2|CE|20820^20820^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rTCD|20820^^99ROC|^1^:^1\rINV|2082001|OK^^HL70383~CURRENT^^99ROC|R1|846|1|18||||||20231031||||590647\rINV|2082001|OK^^HL70383~CURRENT^^99ROC|R3|846|1|18||||||20231031||||590647\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134215|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|43|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|29~12|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20\r221005135234||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.87 - 549|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135234||||||||||RSLT\rOBR|10|""||21130^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|21130^21130^99ROC^^^IHELAW|1|76.9|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|2|CE|21130^21130^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rTCD|21130^^99ROC|^1^:^1\rINV|2113001|OK^^HL70383~CURRENT^^99ROC|R1|4123|1|19||||||20230630||||643364\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134223|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|46|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|32~15|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|8.85 - 885|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135242||||||||||RSLT\rOBR|11|""||21170^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|21170^21170^99ROC^^^IHELAW|1|5.15|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|2|CE|21170^21170^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rTCD|21170^^99ROC|^1^:^1\rINV|2117001|OK^^HL70383~CURRENT^^99ROC|R1|298|1|7||||||20230430||||627564\rINV|2117001|OK^^HL70383~CURRENT^^99ROC|R3|298|1|7||||||20230430||||627564\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134231|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|47|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|33~16|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|0.200 - 25.0|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1|-9999999 - 9999999|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135250||||||||||RSLT\rOBR|12|""||21191^^99ROC|||||||\rORC|SC||||CM\rTQ1|||||||||R^^HL70485\rOBX|1|NM|21191^21191^99ROC^^^IHELAW|1|40.8|mg/dL^^99ROC||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|2|CE|21191^21191^99ROC^^^IHELAW|1|^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rTCD|21191^^99ROC|^1^:^1\rINV|2119001|OK^^HL70383~CURRENT^^99ROC|R1|17429|1|24||||||20230630||||663424\rINV|2119001|OK^^HL70383~CURRENT^^99ROC|R3|17429|1|24||||||20230630||||663424\rOBX|3|DTM|PT^Pipetting_Time^99ROC^S_OTHER^Other Supplemental^IHELAW|1|20221005134239|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|4|EI|CalibrationID^CalibrationID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|50|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|5|EI|QCTID^QC Test ID^99ROC^S_OTHER^Other Supplemental^IHELAW|1|46~44|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|6|CE|QCSTATE^QC Status^99ROC^S_OTHER^Other Supplemental^IHELAW|1|1^^99ROC|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|7|ST|TR_TECHNICALLIMIT^TR_TECHNICALLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1|3.00 - 240|||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|8|ST|TR_REPEATLIMIT^TR_REPEATLIMIT^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\rOBX|9|ST|TR_EXPECTEDVALUES^TR_EXPECTEDVALUES^99ROC^S_OTHER^Other Supplemental^IHELAW|1| - |||N^^HL70078|||F|||||ADMIN~BATCH||c303^ROCHE~2272-02^ROCHE~1^ROCHE|20221005135258||||||||||RSLT\r\x1c\r
	return nil
}

func GetHL7MsgType_MeasurementResultsReadyACK() (string, error) {
	msg := hl7.NewMessage(nil)

	//SendStringQueue
	msh := hl7_segments.HL7MSH{}
	msh.CreateFromMessage(msg, HL7MessageTypesDefinitions.HL7MsgType_InventoryRequest, "", "")
	equ := hl7_segments.HL7EQU{}
	equ.CreateSegment()
	hl7Message, err := encodeHL7Objects(&msh, &equ)
	if err != nil {
		return "", err
	}

	return getHL7Packet(hl7Message), nil
	//HL7MsgType_InventoryRequest
}

type HL7TreeBuilderLocation struct {
	FromPos int
	ToPos   int
	Seg     hl7.Segment
}
type HL7TreePositions struct {
	Key      string
	StartPos int
	EndPos   int
}
type HL7TreeBuilder struct {
	Repetitions int
	Positions   []HL7TreeBuilderLocation
}
type HL7SubSegments struct {
	MainSegment hl7.Segment
	SubSegments map[string]interface{}
}

func HL7AddNewPositionForKey(key string, newPosition int, positions []HL7TreePositions) []HL7TreePositions {
	found := false
	for i := 0; i < len(positions); i++ {
		if positions[i].Key == key {
			found = true
			el := positions[i]
			el.EndPos = newPosition
			positions[i] = el
		}
	}
	if !found {
		positions = append(positions, HL7TreePositions{Key: key, StartPos: newPosition, EndPos: newPosition})
	}

	return positions
}

// AllSegments returns the first matching segmane with name s
func HL7MapAllSegments(segments []hl7.Segment) (map[string]interface{}, error) {
	elements := map[string]HL7TreeBuilder{}
	positions := []HL7TreePositions{}
	var subsegs []hl7.Segment

	for i, seg := range segments {
		fld, err := seg.Field(0)
		if err != nil {
			continue
		}
		key := string(fld.Value)

		positions = HL7AddNewPositionForKey(key, i, positions)

		el, ok := elements[key]
		if !ok {
			el = HL7TreeBuilder{Repetitions: 0, Positions: []HL7TreeBuilderLocation{}}
		}
		el.Repetitions++
		el.Positions = append(el.Positions, HL7TreeBuilderLocation{FromPos: i, ToPos: 0, Seg: seg})

		if el.Repetitions > 1 {
			el.Positions[el.Repetitions-2].ToPos = i - 1
		}
		elements[key] = el
	}

	//now I only take the positions start->end and ignore the ones between

	resp := map[string]interface{}{}
	lastCheckedPos := 0
	lastCheckedIdx := 0

	currentPostions := map[int]HL7TreePositions{}

	for idx, posEl := range positions {
		//ignore subtree
		if posEl.StartPos < lastCheckedPos {
			lastElOk := positions[lastCheckedIdx]
			if lastElOk.EndPos < posEl.EndPos {
				lastElOk.EndPos = posEl.EndPos
				lastCheckedPos = lastElOk.EndPos
			}
			positions[lastCheckedIdx] = lastElOk
			currentPostions[lastCheckedIdx] = lastElOk

			//now update this position in the elemets array so when we parse them we will get the full data
			tmpEl := elements[lastElOk.Key]
			tmpElPos := tmpEl.Positions[len(tmpEl.Positions)-1]
			tmpElPos.ToPos = lastElOk.EndPos
			tmpEl.Positions[len(tmpEl.Positions)-1] = tmpElPos
			elements[lastElOk.Key] = tmpEl
			continue
		}
		lastCheckedPos = posEl.EndPos
		lastCheckedIdx = idx
		currentPostions[idx] = posEl
	}

	for _, posEl := range currentPostions {

		key := posEl.Key
		tb := elements[key]
		if tb.Repetitions == 1 {
			resp[key] = tb.Positions[0].Seg
		} else {
			resp[key] = []HL7SubSegments{}
			for _, el := range tb.Positions {

				if el.ToPos > 0 {
					subsegs = segments[el.FromPos : el.ToPos+1]
				} else {
					subsegs = segments[el.FromPos:]
				}

				calculatedSubSeg, err := HL7MapAllSegments(subsegs)
				if err != nil {
					return nil, err
				}
				resp[key] = append(resp[key].([]HL7SubSegments), HL7SubSegments{MainSegment: el.Seg, SubSegments: calculatedSubSeg})
			}
		}
	}
	return resp, nil
}

func printMap(indent int, obj map[string]interface{}) {
	for key, data := range obj {
		if dataSeg, ok := data.(hl7.Segment); ok {
			fmt.Printf("\n%s%s    -    %s", strings.Repeat(" ", indent), key, dataSeg.Value)
		} else {

			if arrHLTSubegments, ok := data.([]HL7SubSegments); ok {
				fmt.Printf("\n%s%s[", strings.Repeat(" ", indent), key)
				for _, subSeg := range arrHLTSubegments {
					//mainSeg, _ := subSeg.MainSegment.Field(0)

					///fmt.Printf("\n%s%s", strings.Repeat(" ", indent+3), mainSeg.Value)

					fmt.Printf("\n%s{", strings.Repeat(" ", indent+3))
					printMap(indent+6, subSeg.SubSegments)
					fmt.Printf("\n%s}", strings.Repeat(" ", indent+3))
				}
				fmt.Printf("\n%s]", strings.Repeat(" ", indent))
			}

		}
	}
}

func returnSegmentFromMap(segName string, res map[string]interface{}) hl7.Segment {
	result := hl7.Segment{}
	if seg, ok := res[segName]; ok {

		if segHL7, ok := seg.(hl7.Segment); ok {
			result = segHL7
		} else {
			if segArrHL7, ok := seg.([]HL7SubSegments); ok {
				result = segArrHL7[0].MainSegment
			}
		}

	}
	return result
}
