"use strict";(()=>{var bt={h:3600,m:60,s:1};function St(r){let t=r.trim().toLowerCase();if(t==="")return 0;let e=/(\d+)\s*([hms])/g,s=0,i=0,n;for(;(n=e.exec(t))!==null;)s+=parseInt(n[1],10)*bt[n[2]],i+=n[0].length;if(i===0){let o=parseInt(t,10);return Number.isFinite(o)?o:0}return s}function xt(r){let t=Math.max(0,Math.floor(r)),e=Math.floor(t/3600),s=Math.floor(t%3600/60),i=t%60,n=o=>o.toString().padStart(2,"0");return{hh:n(e),mm:n(s),ss:n(i)}}var wt=`
<style>
  :host {
    display: inline-flex;
    gap: 0.5rem;
    font-family: var(--wc-counter-font, ui-monospace, monospace);
    font-variant-numeric: tabular-nums;
    color: var(--wc-counter-color, #fff);
  }
  .cell {
    display: flex;
    flex-direction: column;
    align-items: center;
    min-width: 3.25rem;
    padding: 0.5rem 0.75rem;
    border-radius: 0.75rem;
    background: var(--wc-counter-bg, rgba(0, 0, 0, 0.55));
    box-shadow: 0 1px 0 rgba(255, 255, 255, 0.08) inset,
                0 8px 24px rgba(0, 0, 0, 0.25);
  }
  .num {
    font-size: var(--wc-counter-num-size, 2rem);
    font-weight: 700;
    line-height: 1;
    letter-spacing: -0.02em;
  }
  .lbl {
    margin-top: 0.25rem;
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    opacity: 0.7;
  }
  :host([done]) .cell {
    background: var(--wc-counter-bg-done, rgba(185, 28, 28, 0.85));
  }
</style>
<div class="cell"><span class="num" data-slot="hh">00</span><span class="lbl">hours</span></div>
<div class="cell"><span class="num" data-slot="mm">00</span><span class="lbl">min</span></div>
<div class="cell"><span class="num" data-slot="ss">00</span><span class="lbl">sec</span></div>
`,D=class extends HTMLElement{constructor(){super();this.remaining=0;this.timerId=null;let e=this.attachShadow({mode:"open"});e.innerHTML=wt,this.hhEl=e.querySelector('[data-slot="hh"]'),this.mmEl=e.querySelector('[data-slot="mm"]'),this.ssEl=e.querySelector('[data-slot="ss"]')}static get observedAttributes(){return["from"]}connectedCallback(){this.reset(),this.start()}disconnectedCallback(){this.stop()}attributeChangedCallback(e){e==="from"&&(this.reset(),this.isConnected&&this.start())}reset(){this.remaining=St(this.getAttribute("from")??""),this.render(),this.toggleAttribute("done",this.remaining<=0)}start(){this.stop(),!(this.remaining<=0)&&(this.timerId=window.setInterval(()=>this.tick(),1e3))}stop(){this.timerId!==null&&(window.clearInterval(this.timerId),this.timerId=null)}tick(){this.remaining-=1,this.remaining<=0&&(this.remaining=0,this.stop(),this.toggleAttribute("done",!0),this.dispatchEvent(new CustomEvent("wc-counter:done",{bubbles:!0,composed:!0}))),this.render()}render(){let{hh:e,mm:s,ss:i}=xt(this.remaining);this.hhEl.textContent=e,this.mmEl.textContent=s,this.ssEl.textContent=i}};customElements.get("wc-counter")||customElements.define("wc-counter",D);var k=globalThis,I=k.ShadowRoot&&(k.ShadyCSS===void 0||k.ShadyCSS.nativeShadow)&&"adoptedStyleSheets"in Document.prototype&&"replace"in CSSStyleSheet.prototype,j=Symbol(),Q=new WeakMap,C=class{constructor(t,e,s){if(this._$cssResult$=!0,s!==j)throw Error("CSSResult is not constructable. Use `unsafeCSS` or `css` instead.");this.cssText=t,this.t=e}get styleSheet(){let t=this.o,e=this.t;if(I&&t===void 0){let s=e!==void 0&&e.length===1;s&&(t=Q.get(e)),t===void 0&&((this.o=t=new CSSStyleSheet).replaceSync(this.cssText),s&&Q.set(e,t))}return t}toString(){return this.cssText}},tt=r=>new C(typeof r=="string"?r:r+"",void 0,j),B=(r,...t)=>{let e=r.length===1?r[0]:t.reduce((s,i,n)=>s+(o=>{if(o._$cssResult$===!0)return o.cssText;if(typeof o=="number")return o;throw Error("Value passed to 'css' function must be a 'css' function result: "+o+". Use 'unsafeCSS' to pass non-literal values, but take care to ensure page security.")})(i)+r[n+1],r[0]);return new C(e,r,j)},et=(r,t)=>{if(I)r.adoptedStyleSheets=t.map(e=>e instanceof CSSStyleSheet?e:e.styleSheet);else for(let e of t){let s=document.createElement("style"),i=k.litNonce;i!==void 0&&s.setAttribute("nonce",i),s.textContent=e.cssText,r.appendChild(s)}},q=I?r=>r:r=>r instanceof CSSStyleSheet?(t=>{let e="";for(let s of t.cssRules)e+=s.cssText;return tt(e)})(r):r;var{is:Ct,defineProperty:Mt,getOwnPropertyDescriptor:Pt,getOwnPropertyNames:Tt,getOwnPropertySymbols:Ut,getPrototypeOf:Ht}=Object,g=globalThis,st=g.trustedTypes,Nt=st?st.emptyScript:"",Ot=g.reactiveElementPolyfillSupport,M=(r,t)=>r,F={toAttribute(r,t){switch(t){case Boolean:r=r?Nt:null;break;case Object:case Array:r=r==null?r:JSON.stringify(r)}return r},fromAttribute(r,t){let e=r;switch(t){case Boolean:e=r!==null;break;case Number:e=r===null?null:Number(r);break;case Object:case Array:try{e=JSON.parse(r)}catch{e=null}}return e}},rt=(r,t)=>!Ct(r,t),it={attribute:!0,type:String,converter:F,reflect:!1,useDefault:!1,hasChanged:rt};Symbol.metadata??(Symbol.metadata=Symbol("metadata")),g.litPropertyMetadata??(g.litPropertyMetadata=new WeakMap);var f=class extends HTMLElement{static addInitializer(t){this._$Ei(),(this.l??(this.l=[])).push(t)}static get observedAttributes(){return this.finalize(),this._$Eh&&[...this._$Eh.keys()]}static createProperty(t,e=it){if(e.state&&(e.attribute=!1),this._$Ei(),this.prototype.hasOwnProperty(t)&&((e=Object.create(e)).wrapped=!0),this.elementProperties.set(t,e),!e.noAccessor){let s=Symbol(),i=this.getPropertyDescriptor(t,s,e);i!==void 0&&Mt(this.prototype,t,i)}}static getPropertyDescriptor(t,e,s){let{get:i,set:n}=Pt(this.prototype,t)??{get(){return this[e]},set(o){this[e]=o}};return{get:i,set(o){let l=i?.call(this);n?.call(this,o),this.requestUpdate(t,l,s)},configurable:!0,enumerable:!0}}static getPropertyOptions(t){return this.elementProperties.get(t)??it}static _$Ei(){if(this.hasOwnProperty(M("elementProperties")))return;let t=Ht(this);t.finalize(),t.l!==void 0&&(this.l=[...t.l]),this.elementProperties=new Map(t.elementProperties)}static finalize(){if(this.hasOwnProperty(M("finalized")))return;if(this.finalized=!0,this._$Ei(),this.hasOwnProperty(M("properties"))){let e=this.properties,s=[...Tt(e),...Ut(e)];for(let i of s)this.createProperty(i,e[i])}let t=this[Symbol.metadata];if(t!==null){let e=litPropertyMetadata.get(t);if(e!==void 0)for(let[s,i]of e)this.elementProperties.set(s,i)}this._$Eh=new Map;for(let[e,s]of this.elementProperties){let i=this._$Eu(e,s);i!==void 0&&this._$Eh.set(i,e)}this.elementStyles=this.finalizeStyles(this.styles)}static finalizeStyles(t){let e=[];if(Array.isArray(t)){let s=new Set(t.flat(1/0).reverse());for(let i of s)e.unshift(q(i))}else t!==void 0&&e.push(q(t));return e}static _$Eu(t,e){let s=e.attribute;return s===!1?void 0:typeof s=="string"?s:typeof t=="string"?t.toLowerCase():void 0}constructor(){super(),this._$Ep=void 0,this.isUpdatePending=!1,this.hasUpdated=!1,this._$Em=null,this._$Ev()}_$Ev(){this._$ES=new Promise(t=>this.enableUpdating=t),this._$AL=new Map,this._$E_(),this.requestUpdate(),this.constructor.l?.forEach(t=>t(this))}addController(t){(this._$EO??(this._$EO=new Set)).add(t),this.renderRoot!==void 0&&this.isConnected&&t.hostConnected?.()}removeController(t){this._$EO?.delete(t)}_$E_(){let t=new Map,e=this.constructor.elementProperties;for(let s of e.keys())this.hasOwnProperty(s)&&(t.set(s,this[s]),delete this[s]);t.size>0&&(this._$Ep=t)}createRenderRoot(){let t=this.shadowRoot??this.attachShadow(this.constructor.shadowRootOptions);return et(t,this.constructor.elementStyles),t}connectedCallback(){this.renderRoot??(this.renderRoot=this.createRenderRoot()),this.enableUpdating(!0),this._$EO?.forEach(t=>t.hostConnected?.())}enableUpdating(t){}disconnectedCallback(){this._$EO?.forEach(t=>t.hostDisconnected?.())}attributeChangedCallback(t,e,s){this._$AK(t,s)}_$ET(t,e){let s=this.constructor.elementProperties.get(t),i=this.constructor._$Eu(t,s);if(i!==void 0&&s.reflect===!0){let n=(s.converter?.toAttribute!==void 0?s.converter:F).toAttribute(e,s.type);this._$Em=t,n==null?this.removeAttribute(i):this.setAttribute(i,n),this._$Em=null}}_$AK(t,e){let s=this.constructor,i=s._$Eh.get(t);if(i!==void 0&&this._$Em!==i){let n=s.getPropertyOptions(i),o=typeof n.converter=="function"?{fromAttribute:n.converter}:n.converter?.fromAttribute!==void 0?n.converter:F;this._$Em=i;let l=o.fromAttribute(e,n.type);this[i]=l??this._$Ej?.get(i)??l,this._$Em=null}}requestUpdate(t,e,s,i=!1,n){if(t!==void 0){let o=this.constructor;if(i===!1&&(n=this[t]),s??(s=o.getPropertyOptions(t)),!((s.hasChanged??rt)(n,e)||s.useDefault&&s.reflect&&n===this._$Ej?.get(t)&&!this.hasAttribute(o._$Eu(t,s))))return;this.C(t,e,s)}this.isUpdatePending===!1&&(this._$ES=this._$EP())}C(t,e,{useDefault:s,reflect:i,wrapped:n},o){s&&!(this._$Ej??(this._$Ej=new Map)).has(t)&&(this._$Ej.set(t,o??e??this[t]),n!==!0||o!==void 0)||(this._$AL.has(t)||(this.hasUpdated||s||(e=void 0),this._$AL.set(t,e)),i===!0&&this._$Em!==t&&(this._$Eq??(this._$Eq=new Set)).add(t))}async _$EP(){this.isUpdatePending=!0;try{await this._$ES}catch(e){Promise.reject(e)}let t=this.scheduleUpdate();return t!=null&&await t,!this.isUpdatePending}scheduleUpdate(){return this.performUpdate()}performUpdate(){if(!this.isUpdatePending)return;if(!this.hasUpdated){if(this.renderRoot??(this.renderRoot=this.createRenderRoot()),this._$Ep){for(let[i,n]of this._$Ep)this[i]=n;this._$Ep=void 0}let s=this.constructor.elementProperties;if(s.size>0)for(let[i,n]of s){let{wrapped:o}=n,l=this[i];o!==!0||this._$AL.has(i)||l===void 0||this.C(i,void 0,n,l)}}let t=!1,e=this._$AL;try{t=this.shouldUpdate(e),t?(this.willUpdate(e),this._$EO?.forEach(s=>s.hostUpdate?.()),this.update(e)):this._$EM()}catch(s){throw t=!1,this._$EM(),s}t&&this._$AE(e)}willUpdate(t){}_$AE(t){this._$EO?.forEach(e=>e.hostUpdated?.()),this.hasUpdated||(this.hasUpdated=!0,this.firstUpdated(t)),this.updated(t)}_$EM(){this._$AL=new Map,this.isUpdatePending=!1}get updateComplete(){return this.getUpdateComplete()}getUpdateComplete(){return this._$ES}shouldUpdate(t){return!0}update(t){this._$Eq&&(this._$Eq=this._$Eq.forEach(e=>this._$ET(e,this[e]))),this._$EM()}updated(t){}firstUpdated(t){}};f.elementStyles=[],f.shadowRootOptions={mode:"open"},f[M("elementProperties")]=new Map,f[M("finalized")]=new Map,Ot?.({ReactiveElement:f}),(g.reactiveElementVersions??(g.reactiveElementVersions=[])).push("2.1.2");var T=globalThis,nt=r=>r,z=T.trustedTypes,ot=z?z.createPolicy("lit-html",{createHTML:r=>r}):void 0,pt="$lit$",_=`lit$${Math.random().toFixed(9).slice(2)}$`,ut="?"+_,Rt=`<${ut}>`,E=document,U=()=>E.createComment(""),H=r=>r===null||typeof r!="object"&&typeof r!="function",Z=Array.isArray,Lt=r=>Z(r)||typeof r?.[Symbol.iterator]=="function",V=`[ 	
\f\r]`,P=/<(?:(!--|\/[^a-zA-Z])|(\/?[a-zA-Z][^>\s]*)|(\/?$))/g,at=/-->/g,ht=/>/g,v=RegExp(`>|${V}(?:([^\\s"'>=/]+)(${V}*=${V}*(?:[^ 	
\f\r"'\`<>=]|("|')|))|$)`,"g"),lt=/'/g,ct=/"/g,mt=/^(?:script|style|textarea|title)$/i,G=r=>(t,...e)=>({_$litType$:r,strings:t,values:e}),ft=G(1),Wt=G(2),Jt=G(3),b=Symbol.for("lit-noChange"),p=Symbol.for("lit-nothing"),dt=new WeakMap,A=E.createTreeWalker(E,129);function $t(r,t){if(!Z(r)||!r.hasOwnProperty("raw"))throw Error("invalid template strings array");return ot!==void 0?ot.createHTML(t):t}var kt=(r,t)=>{let e=r.length-1,s=[],i,n=t===2?"<svg>":t===3?"<math>":"",o=P;for(let l=0;l<e;l++){let a=r[l],c,d,h=-1,u=0;for(;u<a.length&&(o.lastIndex=u,d=o.exec(a),d!==null);)u=o.lastIndex,o===P?d[1]==="!--"?o=at:d[1]!==void 0?o=ht:d[2]!==void 0?(mt.test(d[2])&&(i=RegExp("</"+d[2],"g")),o=v):d[3]!==void 0&&(o=v):o===v?d[0]===">"?(o=i??P,h=-1):d[1]===void 0?h=-2:(h=o.lastIndex-d[2].length,c=d[1],o=d[3]===void 0?v:d[3]==='"'?ct:lt):o===ct||o===lt?o=v:o===at||o===ht?o=P:(o=v,i=void 0);let m=o===v&&r[l+1].startsWith("/>")?" ":"";n+=o===P?a+Rt:h>=0?(s.push(c),a.slice(0,h)+pt+a.slice(h)+_+m):a+_+(h===-2?l:m)}return[$t(r,n+(r[e]||"<?>")+(t===2?"</svg>":t===3?"</math>":"")),s]},N=class r{constructor({strings:t,_$litType$:e},s){let i;this.parts=[];let n=0,o=0,l=t.length-1,a=this.parts,[c,d]=kt(t,e);if(this.el=r.createElement(c,s),A.currentNode=this.el.content,e===2||e===3){let h=this.el.content.firstChild;h.replaceWith(...h.childNodes)}for(;(i=A.nextNode())!==null&&a.length<l;){if(i.nodeType===1){if(i.hasAttributes())for(let h of i.getAttributeNames())if(h.endsWith(pt)){let u=d[o++],m=i.getAttribute(h).split(_),S=/([.?@])?(.*)/.exec(u);a.push({type:1,index:n,name:S[2],strings:m,ctor:S[1]==="."?W:S[1]==="?"?J:S[1]==="@"?X:w}),i.removeAttribute(h)}else h.startsWith(_)&&(a.push({type:6,index:n}),i.removeAttribute(h));if(mt.test(i.tagName)){let h=i.textContent.split(_),u=h.length-1;if(u>0){i.textContent=z?z.emptyScript:"";for(let m=0;m<u;m++)i.append(h[m],U()),A.nextNode(),a.push({type:2,index:++n});i.append(h[u],U())}}}else if(i.nodeType===8)if(i.data===ut)a.push({type:2,index:n});else{let h=-1;for(;(h=i.data.indexOf(_,h+1))!==-1;)a.push({type:7,index:n}),h+=_.length-1}n++}}static createElement(t,e){let s=E.createElement("template");return s.innerHTML=t,s}};function x(r,t,e=r,s){if(t===b)return t;let i=s!==void 0?e._$Co?.[s]:e._$Cl,n=H(t)?void 0:t._$litDirective$;return i?.constructor!==n&&(i?._$AO?.(!1),n===void 0?i=void 0:(i=new n(r),i._$AT(r,e,s)),s!==void 0?(e._$Co??(e._$Co=[]))[s]=i:e._$Cl=i),i!==void 0&&(t=x(r,i._$AS(r,t.values),i,s)),t}var K=class{constructor(t,e){this._$AV=[],this._$AN=void 0,this._$AD=t,this._$AM=e}get parentNode(){return this._$AM.parentNode}get _$AU(){return this._$AM._$AU}u(t){let{el:{content:e},parts:s}=this._$AD,i=(t?.creationScope??E).importNode(e,!0);A.currentNode=i;let n=A.nextNode(),o=0,l=0,a=s[0];for(;a!==void 0;){if(o===a.index){let c;a.type===2?c=new O(n,n.nextSibling,this,t):a.type===1?c=new a.ctor(n,a.name,a.strings,this,t):a.type===6&&(c=new Y(n,this,t)),this._$AV.push(c),a=s[++l]}o!==a?.index&&(n=A.nextNode(),o++)}return A.currentNode=E,i}p(t){let e=0;for(let s of this._$AV)s!==void 0&&(s.strings!==void 0?(s._$AI(t,s,e),e+=s.strings.length-2):s._$AI(t[e])),e++}},O=class r{get _$AU(){return this._$AM?._$AU??this._$Cv}constructor(t,e,s,i){this.type=2,this._$AH=p,this._$AN=void 0,this._$AA=t,this._$AB=e,this._$AM=s,this.options=i,this._$Cv=i?.isConnected??!0}get parentNode(){let t=this._$AA.parentNode,e=this._$AM;return e!==void 0&&t?.nodeType===11&&(t=e.parentNode),t}get startNode(){return this._$AA}get endNode(){return this._$AB}_$AI(t,e=this){t=x(this,t,e),H(t)?t===p||t==null||t===""?(this._$AH!==p&&this._$AR(),this._$AH=p):t!==this._$AH&&t!==b&&this._(t):t._$litType$!==void 0?this.$(t):t.nodeType!==void 0?this.T(t):Lt(t)?this.k(t):this._(t)}O(t){return this._$AA.parentNode.insertBefore(t,this._$AB)}T(t){this._$AH!==t&&(this._$AR(),this._$AH=this.O(t))}_(t){this._$AH!==p&&H(this._$AH)?this._$AA.nextSibling.data=t:this.T(E.createTextNode(t)),this._$AH=t}$(t){let{values:e,_$litType$:s}=t,i=typeof s=="number"?this._$AC(t):(s.el===void 0&&(s.el=N.createElement($t(s.h,s.h[0]),this.options)),s);if(this._$AH?._$AD===i)this._$AH.p(e);else{let n=new K(i,this),o=n.u(this.options);n.p(e),this.T(o),this._$AH=n}}_$AC(t){let e=dt.get(t.strings);return e===void 0&&dt.set(t.strings,e=new N(t)),e}k(t){Z(this._$AH)||(this._$AH=[],this._$AR());let e=this._$AH,s,i=0;for(let n of t)i===e.length?e.push(s=new r(this.O(U()),this.O(U()),this,this.options)):s=e[i],s._$AI(n),i++;i<e.length&&(this._$AR(s&&s._$AB.nextSibling,i),e.length=i)}_$AR(t=this._$AA.nextSibling,e){for(this._$AP?.(!1,!0,e);t!==this._$AB;){let s=nt(t).nextSibling;nt(t).remove(),t=s}}setConnected(t){this._$AM===void 0&&(this._$Cv=t,this._$AP?.(t))}},w=class{get tagName(){return this.element.tagName}get _$AU(){return this._$AM._$AU}constructor(t,e,s,i,n){this.type=1,this._$AH=p,this._$AN=void 0,this.element=t,this.name=e,this._$AM=i,this.options=n,s.length>2||s[0]!==""||s[1]!==""?(this._$AH=Array(s.length-1).fill(new String),this.strings=s):this._$AH=p}_$AI(t,e=this,s,i){let n=this.strings,o=!1;if(n===void 0)t=x(this,t,e,0),o=!H(t)||t!==this._$AH&&t!==b,o&&(this._$AH=t);else{let l=t,a,c;for(t=n[0],a=0;a<n.length-1;a++)c=x(this,l[s+a],e,a),c===b&&(c=this._$AH[a]),o||(o=!H(c)||c!==this._$AH[a]),c===p?t=p:t!==p&&(t+=(c??"")+n[a+1]),this._$AH[a]=c}o&&!i&&this.j(t)}j(t){t===p?this.element.removeAttribute(this.name):this.element.setAttribute(this.name,t??"")}},W=class extends w{constructor(){super(...arguments),this.type=3}j(t){this.element[this.name]=t===p?void 0:t}},J=class extends w{constructor(){super(...arguments),this.type=4}j(t){this.element.toggleAttribute(this.name,!!t&&t!==p)}},X=class extends w{constructor(t,e,s,i,n){super(t,e,s,i,n),this.type=5}_$AI(t,e=this){if((t=x(this,t,e,0)??p)===b)return;let s=this._$AH,i=t===p&&s!==p||t.capture!==s.capture||t.once!==s.once||t.passive!==s.passive,n=t!==p&&(s===p||i);i&&this.element.removeEventListener(this.name,this,s),n&&this.element.addEventListener(this.name,this,t),this._$AH=t}handleEvent(t){typeof this._$AH=="function"?this._$AH.call(this.options?.host??this.element,t):this._$AH.handleEvent(t)}},Y=class{constructor(t,e,s){this.element=t,this.type=6,this._$AN=void 0,this._$AM=e,this.options=s}get _$AU(){return this._$AM._$AU}_$AI(t){x(this,t)}};var It=T.litHtmlPolyfillSupport;It?.(N,O),(T.litHtmlVersions??(T.litHtmlVersions=[])).push("3.3.2");var gt=(r,t,e)=>{let s=e?.renderBefore??t,i=s._$litPart$;if(i===void 0){let n=e?.renderBefore??null;s._$litPart$=i=new O(t.insertBefore(U(),n),n,void 0,e??{})}return i._$AI(r),i};var R=globalThis,y=class extends f{constructor(){super(...arguments),this.renderOptions={host:this},this._$Do=void 0}createRenderRoot(){var e;let t=super.createRenderRoot();return(e=this.renderOptions).renderBefore??(e.renderBefore=t.firstChild),t}update(t){let e=this.render();this.hasUpdated||(this.renderOptions.isConnected=this.isConnected),super.update(t),this._$Do=gt(e,this.renderRoot,this.renderOptions)}connectedCallback(){super.connectedCallback(),this._$Do?.setConnected(!0)}disconnectedCallback(){super.disconnectedCallback(),this._$Do?.setConnected(!1)}render(){return b}};y._$litElement$=!0,y.finalized=!0,R.litElementHydrateSupport?.({LitElement:y});var zt=R.litElementPolyfillSupport;zt?.({LitElement:y});(R.litElementVersions??(R.litElementVersions=[])).push("4.2.2");function Dt(r){return r[Math.random()*r.length|0]}function jt(r){return[...r]}var L=class extends y{constructor(){super();this.layer=null;this.characters="\u2728",this.particles=10,this.radius=0}render(){return ft`
      <slot></slot>
      <div class="layer"></div>
    `}firstUpdated(e){this.layer=this.renderRoot.querySelector(".layer")}emit(){let e=jt(this.characters);if(e.length===0||!this.layer)return;let s=Math.max(1,Math.floor(this.particles)||10),i=this.getBoundingClientRect(),n=i.width,o=i.height,l=n/2,a=o/2,c=this.radius>0?this.radius:Math.max(70,Math.max(n,o)*.9);for(let d=0;d<s;d++){let h=document.createElement("span");h.className="p",h.textContent=Dt(e);let u=Math.random()*Math.PI*2,m=c*(.55+Math.sqrt(Math.random())*.7),S=Math.cos(u)*m,_t=Math.sin(u)*m,yt=40+Math.random()*60,vt=(Math.random()-.5)*180,At=900+Math.random()*500,Et=1+Math.random()*1.3,$=h.style;$.setProperty("--x0",`${l.toFixed(1)}px`),$.setProperty("--y0",`${a.toFixed(1)}px`),$.setProperty("--dx",`${S.toFixed(1)}px`),$.setProperty("--dy",`${_t.toFixed(1)}px`),$.setProperty("--fall",`${yt.toFixed(1)}px`),$.setProperty("--rot",`${vt.toFixed(1)}deg`),$.setProperty("--dur",`${At.toFixed(0)}ms`),$.setProperty("--size",`${Et.toFixed(2)}rem`),h.addEventListener("animationend",()=>h.remove(),{once:!0}),this.layer.appendChild(h)}}};L.styles=B`
    :host {
      position: relative;
      display: inline-block;
    }
    .layer {
      position: absolute;
      inset: 0;
      overflow: visible;
      pointer-events: none;
      z-index: 10;
    }
    .p {
      position: absolute;
      left: var(--x0);
      top: var(--y0);
      line-height: 1;
      pointer-events: none;
      user-select: none;
      font-size: var(--size);
      transform-origin: center;
      will-change: transform, opacity;
      filter: drop-shadow(0 0 6px rgba(255, 220, 120, 0.55));
      /* Linear timing; keyframes encode ease-out burst + ease-in fall. */
      animation: wc-particles-firework var(--dur) linear forwards;
    }
    @keyframes wc-particles-firework {
      0% {
        transform: translate(-50%, -50%) scale(0.3) rotate(0deg);
        opacity: 0;
      }
      6% {
        transform: translate(-50%, -50%) scale(1.35) rotate(0deg);
        opacity: 1;
      }
      30% {
        transform:
          translate(calc(-50% + var(--dx) * 0.78), calc(-50% + var(--dy) * 0.78))
          scale(1)
          rotate(calc(var(--rot) * 0.3));
        opacity: 1;
      }
      60% {
        transform:
          translate(calc(-50% + var(--dx)), calc(-50% + var(--dy) + var(--fall) * 0.15))
          scale(0.95)
          rotate(calc(var(--rot) * 0.6));
        opacity: 1;
      }
      100% {
        transform:
          translate(calc(-50% + var(--dx)), calc(-50% + var(--dy) + var(--fall)))
          scale(0.55)
          rotate(var(--rot));
        opacity: 0;
      }
    }
  `,L.properties={characters:{type:String},particles:{type:Number},radius:{type:Number}};customElements.get("wc-particles")||customElements.define("wc-particles",L);})();
/*! Bundled license information:

@lit/reactive-element/css-tag.js:
  (**
   * @license
   * Copyright 2019 Google LLC
   * SPDX-License-Identifier: BSD-3-Clause
   *)

@lit/reactive-element/reactive-element.js:
lit-html/lit-html.js:
lit-element/lit-element.js:
  (**
   * @license
   * Copyright 2017 Google LLC
   * SPDX-License-Identifier: BSD-3-Clause
   *)

lit-html/is-server.js:
  (**
   * @license
   * Copyright 2022 Google LLC
   * SPDX-License-Identifier: BSD-3-Clause
   *)
*/
