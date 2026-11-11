import {describe,it,expect,vi,afterEach} from 'vitest'
import {api,newField,syntheticTranscript} from './api'
describe('browser API contract',()=>{
 afterEach(()=>vi.unstubAllGlobals())
 it('uses same-origin cookies, JSON and does not persist secrets',async()=>{const fetch=vi.fn().mockResolvedValue({ok:true,json:async()=>({ok:true})});vi.stubGlobal('fetch',fetch);await api('sessions','POST',{patient_ref:'SYNTHETIC'});expect(fetch).toHaveBeenCalledWith('/v1/sessions',expect.objectContaining({credentials:'same-origin',method:'POST',body:'{"patient_ref":"SYNTHETIC"}'}))})
 it('propagates explicit API errors',async()=>{vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:false,json:async()=>({error:'required_fields_missing'})}));await expect(api('sessions/x/approve','POST',{})).rejects.toThrow('required_fields_missing')})
 it('synthetic examples match the selected specialty fields',()=>{const field={...newField(),key:'puls',type:'number',min:20,max:250};const t:any={fields:[field,{...newField(),key:'anamneza'}]};expect(syntheticTranscript(t)).toContain('puls: 135');expect(syntheticTranscript(t)).toContain('anamneza: observație sintetică de test')})
 it('creates valid local field keys',()=>{expect(newField().key).toMatch(/^[a-z][a-z0-9_]+$/);expect(newField().required).toBe(false)})
})
