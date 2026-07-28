package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gongahkia/norbot/internal/domain"
)

// acceptance error preserves browser output for the operator-visible verifier report.
type AcceptanceError struct {
	Output string
	Err    error
}

func (e AcceptanceError) Error() string { return e.Err.Error() }
func (e AcceptanceError) Unwrap() error { return e.Err }

func (d Deployment) runAcceptance(ctx context.Context, docker DockerClient, run domain.Run, network string, contract domain.AcceptanceContract) (map[string]any, error) {
	if err := contract.Validate(); err != nil {
		return nil, err
	}
	image := strings.TrimSpace(d.BrowserImage)
	if image == "" {
		image = "norbot-verifier-browser:local"
	}
	if _, ok := docker.Runner.(InputCommandRunner); !ok {
		return map[string]any{"status": "skipped", "reason": "command runner does not support stdin"}, nil
	}
	input, err := json.Marshal(contract)
	if err != nil {
		return nil, err
	}
	args := []string{"run", "--rm", "-i", "--init", "--read-only", "--user", "10001:10001", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256", "--memory=2g", "--cpus=2", "--shm-size=1g", "--tmpfs", "/tmp:rw,noexec,nosuid,size=128m", "--network", network, "--label", "norbot.managed=true", "--label", "norbot.app_id=" + ApplicationID(run), "--label", "norbot.role=acceptance", image, "node", "-e", acceptanceRunner}
	output, err := docker.RunInput(ctx, string(input), args...)
	if err != nil {
		return nil, AcceptanceError{Output: string(output), Err: fmt.Errorf("run browser acceptance checks: %w", err)}
	}
	var report map[string]any
	if err := json.Unmarshal(output, &report); err != nil {
		return nil, AcceptanceError{Output: string(output), Err: fmt.Errorf("decode browser acceptance result: %w", err)}
	}
	if report["status"] != "pass" {
		encoded, _ := json.Marshal(report)
		return report, AcceptanceError{Output: string(encoded), Err: fmt.Errorf("browser acceptance checks failed")}
	}
	return report, nil
}

const acceptanceRunner = `const fs=require('fs');
const {chromium}=require('playwright');
const c=JSON.parse(fs.readFileSync(0,'utf8'));
const base='http://frontend:8080'; const backend='http://backend:8000';
const out={status:'pass',flows:[],api_contracts:[],accessibility:[],screenshots:[]};
const pages=new Map();
async function pageFor(id){if(!pages.has(id)){const p=await browser.newPage();for(const x of c.seed_data||[]){if(x.kind==='local_storage')await p.addInitScript(([k,v])=>localStorage.setItem(k,v),[x.key,x.value]);}pages.set(id,p);}return pages.get(id)}
function target(p,s){if(s.selector)return p.locator(s.selector);if(s.role&&s.name)return p.getByRole(s.role,{name:s.name});throw new Error(s.kind+' requires selector or role/name')}
async function step(p,s){if(s.kind==='goto')return p.goto(base+s.url);if(s.kind==='click')return target(p,s).click();if(s.kind==='fill'||s.kind==='set_value')return target(p,s).fill(s.value);if(s.kind==='expect_text'){if(s.selector||s.role)return target(p,s).filter({hasText:s.text}).first().waitFor();return p.getByText(s.text,{exact:false}).waitFor()}if(s.kind==='expect_value'){const value=await target(p,s).inputValue();if(value!==s.value)throw new Error('expected value '+s.value+', got '+value);return}if(s.kind==='expect_visible')return target(p,s).waitFor();if(s.kind==='expect_count'){const n=await target(p,s).count();if(n!==s.count)throw new Error('expected '+s.count+' matches for '+(s.selector||s.name)+', got '+n);return}if(s.kind==='expect_attribute'){const attribute=s.attribute||(s.selector?s.name:'');const value=await target(p,s).getAttribute(attribute);if(value!==s.value)throw new Error('expected '+attribute+'='+s.value+', got '+value);return}if(s.kind==='expect_url')return p.waitForURL('**'+s.url);if(s.kind==='reload')return p.reload();if(s.kind==='focus')return target(p,s).focus();if(s.kind==='press_key')return target(p,s).press(s.key);if(s.kind==='local_storage')return p.evaluate(([k,v])=>localStorage.setItem(k,v),[s.key,s.value]);throw new Error('unsupported step '+s.kind)}
let browser;
(async()=>{try{for(const x of c.seed_data||[]){if(x.kind==='http'){const r=await fetch(backend+x.path,{method:x.method,headers:{'content-type':'application/json'},body:x.body||undefined});if(!r.ok)throw new Error('seed '+x.path+' returned '+r.status)}}browser=await chromium.launch({headless:true});for(const f of c.flows||[]){const p=await pageFor(f.id);for(const s of f.steps)await step(p,s);out.flows.push({id:f.id,status:'pass'});}for(const a of c.api_contracts||[]){const r=await fetch(backend+a.path,{method:a.method});const body=await r.text();if(r.status!==a.status||a.body_includes&&!body.includes(a.body_includes))throw new Error('API '+a.id+' failed');out.api_contracts.push({id:a.id,status:'pass',status_code:r.status});}for(const a of c.accessibility||[]){const p=await pageFor(a.flow_id||'home');if(p.url()==='about:blank')await p.goto(base+'/');await p.addScriptTag({path:require.resolve('axe-core')});const scan=await p.evaluate(s=>axe.run(s),a.selector||'body');out.accessibility.push({id:a.id,violations:scan.violations});if(scan.violations.length)throw new Error('accessibility '+a.id+' violations');}for(const s of c.screenshots||[]){const p=await pageFor(s.flow_id||'home');if(s.path)await p.goto(base+s.path);if(p.url()==='about:blank')await p.goto(base+'/');out.screenshots.push({id:s.id,png_base64:(await p.screenshot({fullPage:true,type:'png'})).toString('base64')});}console.log(JSON.stringify(out));}catch(e){out.status='fail';out.error=String(e&&e.stack||e);console.log(JSON.stringify(out));process.exitCode=1;}finally{if(browser)await browser.close();}})();`
