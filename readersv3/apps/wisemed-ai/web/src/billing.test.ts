import {describe,it,expect} from 'vitest'
import {parseMoney,money,emptyPlan} from './billing'
describe('commercial package values',()=>{
 it('preserves currency minor units without binary rounding',()=>{expect(parseMoney('19,99')).toBe(1999);expect(parseMoney('0.01')).toBe(1);expect(parseMoney('20')).toBe(2000)})
 it('rejects negative prices, extra decimals and scientific notation',()=>{for(const value of ['-1','0.001','1e3','NaN',''])expect(()=>parseMoney(value)).toThrow()})
 it('new paid plans are drafts with no invented prices',()=>{const plan=emptyPlan();expect(plan.active).toBe(false);expect(plan.monthly_price_minor).toBe(0);expect(plan.credit_limit_minor).toBe(0);expect(money(1999,'USD')).toContain('19,99')})
})
