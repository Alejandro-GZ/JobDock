// @vitest-environment jsdom
import {render,screen} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {DistributionWidget} from "@/components/distribution-widget";
const summary={count:5,min:1,q1:2,median:3,q3:4,max:10,mean:4,whisker_low:1,whisker_high:4,outliers:[10]};
const items=[{id:1,name:"residual",group:"baseline",samples:[1,2,3,4,10],bins:[{lower:1,upper:5,count:4},{lower:5,upper:10,count:1}],density:[{x:3,density:1},{x:7.5,density:.25}],summary,scores:{psi:.12}}];
describe("DistributionWidget",()=>{beforeEach(()=>{vi.stubGlobal("ResizeObserver",class{constructor(private callback:ResizeObserverCallback){}observe(){this.callback([] as unknown as ResizeObserverEntry[],this as unknown as ResizeObserver)}disconnect(){}})});for(const type of ["histogram","boxplot","violin"] as const)it(`renders ${type} from the reusable summary`,()=>{const view=render(<DistributionWidget type={type} items={items}/>),host=screen.getByLabelText(`${type} distribution`);expect(screen.getByText(/residual/)).toBeTruthy();expect(host.querySelector("svg")?.hasAttribute("preserveAspectRatio")).toBe(false);expect(host.querySelector("svg")?.getAttribute("width")).toBe("620");view.unmount()})});
