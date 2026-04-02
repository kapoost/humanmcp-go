package web

const allTemplates = `
{{define "index.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Author}} — humanMCP</title>
<style>{{template "css" .}}</style>
</head>
<body>
<div class="container">
{{template "header" .}}

{{if .IsOwner}}
<div class="owner-bar">
  <a href="/new" class="btn btn-primary" style="font-size:.9rem;padding:.4rem 1.1rem;text-decoration:none;">+ post</a>
  <a href="/dashboard" style="font-size:.78rem;color:var(--muted);margin-left:auto;text-decoration:none;">stats</a>
</div>
{{end}}

{{if .Pieces}}
<ul class="pieces">
{{range .Pieces}}
<li class="piece-item">
  <div class="piece-meta">
    <span>{{formatDate .Published}}</span>
    {{if ne (lower (print .Access)) "public"}}<span class="locked-badge">{{.Access}}</span>{{end}}
  </div>
  <div class="piece-title">
    <a href="/p/{{.Slug}}">{{.Title}}</a>
    {{if $.IsOwner}}<a href="/edit/{{.Slug}}" class="edit-btn">edit</a>{{end}}
  </div>
  {{if .Description}}<div class="piece-desc">{{.Description}}</div>{{end}}
  <div style="display:flex;align-items:center;gap:.75rem;margin-top:.35rem;flex-wrap:wrap;">
    {{if .Tags}}<div class="tags">{{range .Tags}}<span class="tag">#{{.}}</span>{{end}}</div>{{end}}
  </div>
</li>
{{end}}
</ul>
{{else}}
<div class="empty">Nothing here yet.{{if .IsOwner}} Click &ldquo;+ share&rdquo; to post something.{{end}}</div>
{{end}}

{{template "footer" .}}
</div>
</body></html>
{{end}}

{{define "piece.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Piece.Title}} — {{.Author}}</title>
<style>{{template "css" .}}
.poem-body{font-family:var(--serif);font-size:1.1rem;line-height:2;white-space:pre-wrap;margin:2rem 0;}
.essay-body{font-size:1rem;line-height:1.85;margin:2rem 0;}
.piece-header{margin-bottom:1.5rem;padding-bottom:1rem;border-bottom:1px solid var(--border);}
.piece-type{font-size:.75rem;text-transform:uppercase;letter-spacing:.1em;color:var(--muted);margin-bottom:.5rem;}
.piece-h1{font-size:1.6rem;font-weight:500;line-height:1.3;margin-bottom:.4rem;font-family:var(--serif);}
.piece-info{margin-top:.9rem;padding:.85rem 1rem;border:1px solid var(--border);border-radius:6px;background:var(--tag-bg);display:flex;flex-direction:column;gap:0;}
.status-row{display:grid;grid-template-columns:1.2rem 5.5rem 1fr auto;align-items:start;gap:.4rem .6rem;padding:.5rem 0;border-bottom:1px solid var(--border);font-size:.8rem;}
.status-row:last-of-type{border-bottom:none;}
.status-icon{font-size:.85rem;line-height:1.4;text-align:center;}
.status-key{font-size:.68rem;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);padding-top:.15rem;font-weight:500;}
.status-val{line-height:1.45;color:var(--fg);}
.status-val small{display:block;font-size:.7rem;color:var(--muted);margin-top:.15rem;font-family:monospace;word-break:break-all;}
.status-actions{display:flex;gap:.3rem;align-items:flex-start;padding-top:.1rem;flex-shrink:0;}
.st-active{color:#2e7d32;}
.st-anchored{color:#1a3a8a;}
.st-pending{color:#7a5c00;}
.st-none{color:var(--muted);}
@media(prefers-color-scheme:dark){.st-active{color:#6abf6a;}.st-anchored{color:#8899e0;}.st-pending{color:#d4a017;}}
.info-btn{font-size:.68rem;padding:1px 7px;border:1px solid var(--border);border-radius:3px;background:var(--bg);color:var(--muted);cursor:pointer;text-decoration:none;display:inline-block;white-space:nowrap;}
.info-btn:hover{border-color:var(--accent);color:var(--accent);}
.info-actions{display:flex;gap:.5rem;margin-top:.6rem;flex-wrap:wrap;padding-top:.5rem;border-top:1px solid var(--border);}
.gate-box{background:var(--locked-bg);border:1px solid var(--locked);border-radius:6px;padding:1.25rem;margin:2rem 0;}
.gate-box h3{color:var(--locked);margin-bottom:.75rem;font-size:.95rem;}
.gate-box input[type=text]{width:100%;padding:.5rem;border:1px solid var(--border);border-radius:4px;background:var(--bg);color:var(--fg);margin-bottom:.5rem;font-size:1rem;}
.unlock-success{background:#e8f5e9;border:1px solid #4caf50;border-radius:6px;padding:.75rem 1rem;margin-bottom:1rem;color:#2e7d32;font-size:.85rem;}
</style>
</head>
<body>
<div class="container">
{{template "header-simple" .}}
<a href="/" style="font-size:.85rem;color:var(--muted);display:inline-block;margin-bottom:1.5rem;">&#8592; all pieces</a>
{{with .Piece}}
<div class="piece-header">
  <div class="piece-type">{{.Type}} &middot; {{formatDate .Published}}</div>
  <h1 class="piece-h1">{{.Title}}</h1>
  {{if .Tags}}<div class="tags">{{range .Tags}}<span class="tag">#{{.}}</span>{{end}}</div>{{end}}
  <div class="piece-info">
    {{/* ── Signature row ── */}}
    <div class="status-row">
      {{if .Signature}}
      <span class="status-icon st-active">&#10003;</span>
      <span class="status-key">ed25519</span>
      <span class="status-val">
        active — authorship signed
        <small>{{truncate .Signature 48}}</small>
      </span>
      <span class="status-actions">
        <button class="info-btn" onclick="navigator.clipboard.writeText(this.dataset.v);this.textContent='copied';setTimeout(()=>this.textContent='copy',1500)" data-v="{{.Signature}}">copy</button>
      </span>
      {{else}}
      <span class="status-icon st-none">&#8722;</span>
      <span class="status-key">ed25519</span>
      <span class="status-val st-none">unsigned</span>
      <span></span>
      {{end}}
    </div>
    {{/* ── Bitcoin timestamp row ── */}}
    <div class="status-row">
      {{if eq (otsStatus .OTSProof) "anchored"}}
      <span class="status-icon st-anchored">&#x20BF;</span>
      <span class="status-key">bitcoin</span>
      <span class="status-val st-anchored">
        anchored in Bitcoin blockchain
        <small>hash sent: {{otsHash .}}</small>
        <small>verify: echo "{{otsShort .OTSProof}}…" | base64 -d &gt; piece.ots &amp;&amp; ots verify piece.ots</small>
      </span>
      <span class="status-actions">
        <button class="info-btn" onclick="navigator.clipboard.writeText(this.dataset.v);this.textContent='copied';setTimeout(()=>this.textContent='copy',1500)" data-v="{{.OTSProof}}">copy proof</button>
      </span>
      {{else if eq (otsStatus .OTSProof) "pending"}}
      <span class="status-icon st-pending">&#x20BF;</span>
      <span class="status-key">bitcoin</span>
      <span class="status-val st-pending">
        submitted to calendar — awaiting Bitcoin confirmation (~1hr)
        <small>hash sent: {{otsHash .}}</small>
        <small>proof received: {{otsShort .OTSProof}}…</small>
      </span>
      <span class="status-actions">
        {{if $.IsOwner}}<form method="POST" action="/timestamp/{{.Slug}}" style="display:inline;"><button type="submit" class="info-btn">upgrade</button></form>{{end}}
        <button class="info-btn" onclick="navigator.clipboard.writeText(this.dataset.v);this.textContent='copied';setTimeout(()=>this.textContent='copy',1500)" data-v="{{.OTSProof}}">copy proof</button>
      </span>
      {{else}}
      <span class="status-icon st-none">&#x20BF;</span>
      <span class="status-key">bitcoin</span>
      <span class="status-val st-none">
        not yet timestamped
        <small>hash to send: {{otsHash .}}</small>
      </span>
      <span class="status-actions">
        {{if $.IsOwner}}<form method="POST" action="/timestamp/{{.Slug}}" style="display:inline;"><button type="submit" class="info-btn" style="border-color:var(--accent);color:var(--accent);">submit &#x20BF;</button></form>{{end}}
      </span>
      {{end}}
    </div>
    {{/* ── License row ── */}}
    <div class="status-row">
      <span class="status-icon st-active">&#9670;</span>
      <span class="status-key">license</span>
      <span class="status-val">
        {{if .License}}{{licenseLabel .License}}{{else}}free — read &amp; share with credit{{end}}
        {{if .PriceSats}}<small>{{.PriceSats}} sats for commercial use</small>{{end}}
      </span>
      <span class="status-actions">
        {{if or (eq .License "commercial") (eq .License "exclusive") (eq .License "all-rights")}}
        <a href="/contact?regarding={{.Slug}}" class="info-btn">request</a>
        {{end}}
      </span>
    </div>
    {{/* ── Actions ── */}}
    <div class="info-actions">
      <a href="/contact?regarding={{.Slug}}" class="info-btn">&#9993; leave a message</a>
      <a href="/connect" class="info-btn" style="color:var(--muted);">how to verify &#8599;</a>
    </div>
  </div>
</div>
{{if $.Unlocked}}<div class="unlock-success">&#10003; Correct answer &mdash; content unlocked</div>{{end}}
{{if $.IsLocked}}
  {{if .Description}}<p style="color:var(--muted);margin-bottom:1.5rem;">{{.Description}}</p>{{end}}
  <div class="gate-box">
    <h3>&#128274; This content requires {{.Access}} access</h3>
    {{if eq (print .Gate) "challenge"}}
      <p style="margin-bottom:.75rem;font-size:.9rem;">Answer to read this piece:</p>
      <p style="font-weight:500;margin-bottom:1rem;">{{.Challenge}}</p>
      {{if .Description}}<p style="font-size:.82rem;color:var(--muted);margin-bottom:.75rem;font-style:italic;">Hint: {{.Description}}</p>{{end}}
      {{if $.WrongAnswer}}<p style="color:#c0392b;font-size:.85rem;margin-bottom:.5rem;">&#10007; Wrong answer, try again.</p>{{end}}
      <form method="POST" action="/unlock/{{.Slug}}">
        <input type="text" name="answer" placeholder="Your answer..." autocomplete="off" autofocus>
        <button type="submit" class="btn btn-primary">Unlock</button>
      </form>
    {{else if eq (print .Gate) "manual"}}
      <p style="font-size:.9rem;margin-bottom:1rem;">This piece requires approval. Leave a message explaining why.</p>
      <a href="/contact?regarding={{.Slug}}" class="btn btn-primary">Leave a message</a>
    {{else if eq (print .Gate) "time"}}
      <p style="font-size:.9rem;">This piece is time-locked.</p>
      {{if $.UnlockDate}}<p style="font-weight:500;margin-top:.5rem;">Unlocks: {{$.UnlockDate}}</p>{{end}}
    {{else if eq (print .Gate) "trade"}}
      <p style="font-size:.9rem;margin-bottom:1rem;">Available in exchange for content from your humanMCP server.</p>
      <a href="/contact?regarding={{.Slug}}" class="btn btn-primary">Propose a trade</a>
    {{else}}
      <p style="font-size:.9rem;">Members-only. Contact directly for access.</p>
    {{end}}
  </div>
{{else}}
  <div class="{{if eq .Type "poem"}}poem-body{{else}}essay-body{{end}}">{{nl2br .Body}}</div>
{{end}}
{{end}}
{{template "footer" .}}
</div>
</body></html>
{{end}}

{{define "login.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Owner Login</title>
<style>{{template "css" .}}</style>
</head>
<body>
<div class="container" style="max-width:400px;">
<div style="padding:3rem 0;">
<h1 style="font-size:1.2rem;margin-bottom:1.5rem;">Owner Login</h1>
{{if .}}{{with .Error}}<p style="color:#c0392b;margin-bottom:1rem;font-size:.9rem;">{{.}}</p>{{end}}{{end}}
<form method="POST" action="/login" style="display:grid;gap:.75rem;">
  <input type="password" name="token" placeholder="Edit token" autofocus style="padding:.5rem;border:1px solid var(--border);border-radius:4px;background:var(--bg);color:var(--fg);font-size:1rem;">
  <button type="submit" class="btn btn-primary">Login</button>
</form>
<p style="margin-top:1rem;font-size:.8rem;color:var(--muted);"><a href="/">&#8592; back</a></p>
</div>
</div>
</body></html>
{{end}}

{{define "dashboard.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Dashboard — {{.Author}}</title>
<style>{{template "css" .}}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(110px,1fr));gap:9px;margin-bottom:1.75rem;}
.card{background:var(--accent-light);border:1px solid var(--accent);border-radius:7px;padding:.75rem;}
.card-num{font-size:1.65rem;font-weight:500;color:var(--accent);line-height:1;}
.card-label{font-size:.68rem;color:var(--muted);margin-top:.25rem;}
.section{margin-bottom:1.75rem;}
.section-title{font-size:.7rem;font-weight:500;color:var(--muted);text-transform:uppercase;letter-spacing:.07em;margin-bottom:.6rem;}
.row{display:flex;justify-content:space-between;padding:.35rem 0;border-bottom:1px solid var(--border);font-size:.85rem;}
.row:last-child{border-bottom:none;}
.rv{font-weight:500;color:var(--accent);}
.two-col{display:grid;grid-template-columns:1fr 1fr;gap:1.5rem;}
.hour-bar{display:flex;align-items:flex-end;gap:2px;height:60px;margin-top:.4rem;}
.hb{flex:1;background:var(--accent-light);border-radius:2px 2px 0 0;min-height:2px;}
.ev-list{list-style:none;}
.ev-item{padding:.3rem 0;border-bottom:1px solid var(--border);font-size:.75rem;color:var(--muted);display:flex;gap:.5rem;flex-wrap:wrap;}
.ev-item:last-child{border-bottom:none;}
.ev-type{font-weight:500;color:var(--fg);}
.ba{font-size:.65rem;background:var(--accent-light);color:var(--accent);padding:1px 5px;border-radius:3px;border:1px solid var(--accent);}
.bh{font-size:.65rem;background:var(--tag-bg);color:var(--tag-fg);padding:1px 5px;border-radius:3px;}
.funnel-row{font-size:.82rem;padding:.35rem 0;border-bottom:1px solid var(--border);}
.fp{font-size:.7rem;padding:1px 6px;border-radius:3px;margin-right:.3rem;}
.fp-checked{background:#e3f2fd;color:#1565c0;}
.fp-tried{background:#fff3e0;color:#e65100;}
.fp-unlocked{background:#e8f5e9;color:#2e7d32;}
.msg-preview{padding:.55rem 0;border-bottom:1px solid var(--border);}
.msg-preview:last-child{border-bottom:none;}
</style>
</head>
<body>
<div class="container">
{{template "header-simple" .}}
<div style="display:flex;justify-content:space-between;align-items:baseline;margin-bottom:1.5rem;">
  <h1 style="font-size:1.1rem;font-weight:500;">Dashboard</h1>
  <a href="/" style="font-size:.82rem;color:var(--muted);">&#8592; back</a>
</div>

{{with .Stats}}
<div class="grid">
  <div class="card"><div class="card-num">{{.TotalReads}}</div><div class="card-label">reads</div></div>
  <div class="card"><div class="card-num">{{.UniqueVisitors}}</div><div class="card-label">unique</div></div>
  <div class="card"><div class="card-num">{{.AgentCalls}}</div><div class="card-label">agents</div></div>
  <div class="card"><div class="card-num">{{.HumanVisits}}</div><div class="card-label">humans</div></div>
  <div class="card"><div class="card-num">{{.TotalComments}}</div><div class="card-label">comments</div></div>
  <div class="card"><div class="card-num">{{.TotalMessages}}</div><div class="card-label">messages</div></div>
  <div class="card"><div class="card-num">{{.TotalUnlocks}}</div><div class="card-label">unlocks</div></div>
  <div class="card"><div class="card-num">{{.TotalInterest}}</div><div class="card-label">gate checks</div></div>
</div>

{{if .HourlyReads}}
<div class="section">
  <div class="section-title">reads by hour (UTC)</div>
  <div class="hour-bar" id="hour-bar"></div>
  <div style="display:flex;justify-content:space-between;font-size:.6rem;color:var(--muted);margin-top:2px;"><span>0h</span><span>6h</span><span>12h</span><span>18h</span><span>23h</span></div>
</div>
<script>
(function(){
  var data=[{{range .HourlyReads}}{{.}},{{end}}];
  var max=Math.max.apply(null,data)||1;
  var bar=document.getElementById('hour-bar');
  data.forEach(function(v,i){var d=document.createElement('div');d.className='hb';d.style.height=Math.max(2,Math.round(v/max*58))+'px';d.title='Hour '+i+': '+v+' reads';bar.appendChild(d);});
})();
</script>
{{end}}

<div class="two-col">
<div>
  {{if .ReadsBySlug}}<div class="section"><div class="section-title">reads per piece</div>{{range $s,$n := .ReadsBySlug}}<div class="row"><span>{{$s}}</span><span class="rv">{{$n}}</span></div>{{end}}</div>{{end}}
  {{if .TagReads}}<div class="section"><div class="section-title">reads per tag</div>{{range $t,$n := .TagReads}}<div class="row"><span>#{{$t}}</span><span class="rv">{{$n}}</span></div>{{end}}</div>{{end}}
</div>
<div>
  {{if .ChallengeFunnel}}<div class="section"><div class="section-title">challenge funnel</div>{{range $s,$f := .ChallengeFunnel}}<div class="funnel-row"><div style="font-size:.8rem;font-weight:500;">{{$s}}</div><div style="margin-top:.2rem;"><span class="fp fp-checked">{{index $f 0}} checked</span><span class="fp fp-tried">{{index $f 1}} tried</span><span class="fp fp-unlocked">{{index $f 2}} unlocked</span></div></div>{{end}}</div>{{end}}
  {{if .Countries}}<div class="section"><div class="section-title">by region</div>{{range $c,$n := .Countries}}<div class="row"><span>{{$c}}</span><span class="rv">{{$n}}</span></div>{{end}}</div>{{end}}
  {{if .TopReferrers}}<div class="section"><div class="section-title">referrers</div>{{range $r,$n := .TopReferrers}}<div class="row"><span>{{$r}}</span><span class="rv">{{$n}}</span></div>{{end}}</div>{{end}}
  {{if .TopAgents}}<div class="section"><div class="section-title">visitors</div>{{range $n,$c := .TopAgents}}<div class="row"><span>{{$n}}</span><span class="rv">{{$c}}</span></div>{{end}}</div>{{end}}
</div>
</div>

{{if .RecentEvents}}<div class="section"><div class="section-title">recent activity</div><ul class="ev-list">{{range .RecentEvents}}<li class="ev-item"><span>{{formatDate .At}}</span><span class="ev-type">{{.Type}}</span>{{if eq (print .Caller) "agent"}}<span class="ba">agent</span>{{else if eq (print .Caller) "human"}}<span class="bh">human</span>{{end}}{{if .Slug}}<span style="color:var(--fg);">{{.Slug}}</span>{{end}}{{if .From}}<span>&#8212;{{.From}}</span>{{end}}{{if .Country}}<span>&#127760;{{.Country}}</span>{{end}}</li>{{end}}</ul></div>{{end}}
{{end}}

{{if .Messages}}<div class="section"><div class="section-title">messages &amp; comments ({{len .Messages}})</div>{{range .Messages}}<div class="msg-preview">
  <div style="font-size:.73rem;color:var(--muted);margin-bottom:.3rem;display:flex;gap:.5rem;align-items:center;flex-wrap:wrap;">
    {{if .From}}<strong style="color:var(--fg);font-size:.8rem;">{{.From}}</strong>{{else}}<span>anonymous</span>{{end}}
    <span>{{formatDate .At}}</span>
    {{if .Regarding}}<span style="background:var(--accent-light);color:var(--accent);padding:1px 7px;border-radius:10px;font-size:.7rem;border:1px solid var(--accent);">re: {{.Regarding}}</span>{{end}}
  </div>
  <div style="font-size:.9rem;line-height:1.55;">{{.Text}}</div>
</div>{{end}}</div>
{{else}}<div class="section"><div class="section-title">messages &amp; comments</div><p style="color:var(--muted);font-size:.85rem;">No messages yet.</p></div>{{end}}

{{template "footer" .}}
</div>
</body></html>
{{end}}

{{define "contact.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Contact — {{.Author}}</title>
<style>{{template "css" .}}
textarea{width:100%;padding:.5rem;border:1px solid var(--border);border-radius:4px;background:var(--bg);color:var(--fg);font-family:var(--sans);font-size:.95rem;resize:vertical;line-height:1.6;}
.success-box{background:#e8f5e9;border:1px solid #4caf50;border-radius:6px;padding:1.25rem;color:#2e7d32;}
</style>
</head>
<body>
<div class="container">
{{template "header-simple" .}}
<div style="max-width:520px;">
<h1 style="font-size:1.1rem;font-weight:500;margin-bottom:1.5rem;">Leave a message</h1>
{{if .Sent}}
<div class="success-box"><strong>Message sent.</strong> kapoost will read it.<p style="margin-top:.5rem;font-size:.9rem;">&#8592; <a href="/">back to reading</a></p></div>
{{else}}
{{if .Error}}<p style="color:#c0392b;margin-bottom:1rem;font-size:.85rem;">{{.Error}}</p>{{end}}
<form method="POST" action="/contact" style="display:grid;gap:.75rem;">
  <div><label style="font-size:.82rem;color:var(--muted);display:block;margin-bottom:.3rem;">Name or handle <span style="opacity:.5">(optional)</span></label>
  <input type="text" name="from" maxlength="32" value="{{.From}}" placeholder="anonymous" style="width:100%;padding:.5rem;border:1px solid var(--border);border-radius:4px;background:var(--bg);color:var(--fg);"></div>
  {{if .Pieces}}<div><label style="font-size:.82rem;color:var(--muted);display:block;margin-bottom:.3rem;">About a piece <span style="opacity:.5">(optional)</span></label>
  <select name="regarding" style="width:100%;padding:.5rem;border:1px solid var(--border);border-radius:4px;background:var(--bg);color:var(--fg);">
  <option value="">&#8212; general &#8212;</option>
  {{range .Pieces}}<option value="{{.Slug}}">{{.Title}}</option>{{end}}
  </select></div>{{end}}
  <div><label style="font-size:.82rem;color:var(--muted);display:block;margin-bottom:.3rem;">Message <span style="color:#c0392b">*</span></label>
  <textarea name="text" id="msg-text" maxlength="2000" rows="5" placeholder="Plain text only. No links. Max 2000 characters." oninput="document.getElementById('cc').textContent=this.value.length+'/2000'">{{.Text}}</textarea>
  <div style="font-size:.72rem;color:var(--muted);text-align:right;" id="cc">0/2000</div></div>
  <button type="submit" class="btn btn-primary" style="justify-self:start;">Send</button>
</form>
{{end}}
</div>
{{template "footer" .}}
</div>
</body></html>
{{end}}

{{define "connect.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Connect — {{.Author}}</title>
<style>{{template "css" .}}
.connect-section{margin-bottom:2.5rem;}
.connect-title{font-size:.72rem;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);font-weight:500;margin-bottom:.6rem;}
.code-block{background:var(--tag-bg);border:1px solid var(--border);border-radius:6px;padding:.9rem 1rem;font-family:monospace;font-size:.82rem;line-height:1.7;color:var(--fg);overflow-x:auto;white-space:pre;}
.pill{display:inline-block;font-size:.7rem;background:var(--accent-light);color:var(--accent);padding:2px 8px;border-radius:4px;border:1px solid var(--accent);margin-bottom:.4rem;}
.tool-grid{display:grid;grid-template-columns:1fr 1fr;gap:.5rem;}
.tool-card{padding:.55rem .75rem;border:1px solid var(--border);border-radius:6px;}
.tool-card strong{display:block;font-family:monospace;font-size:.8rem;}
.tool-card span{color:var(--muted);font-size:.75rem;}
</style>
</head>
<body>
<div class="container">
{{template "header-simple" .}}
<a href="/" style="font-size:.85rem;color:var(--muted);display:inline-block;margin-bottom:1.5rem;">&#8592; back</a>
<h1 style="font-size:1.2rem;font-weight:500;margin-bottom:.4rem;">Connect to this humanMCP</h1>
<p style="color:var(--muted);font-size:.88rem;margin-bottom:2rem;">Add {{.Author}}&rsquo;s server to your AI agent. 30 seconds.</p>
<div class="connect-section">
  <div class="connect-title">MCP endpoint</div>
  <div class="code-block" style="word-break:break-all;user-select:all;">https://{{.Domain}}/mcp</div>
</div>
<div class="connect-section">
  <div class="pill">Claude Desktop</div>
  <div class="connect-title">claude_desktop_config.json</div>
  <div class="code-block" style="user-select:all;">{
  "mcpServers": {
    "{{.Author}}": {
      "type": "http",
      "url": "https://{{.Domain}}/mcp"
    }
  }
}</div>
</div>
<div class="connect-section">
  <div class="connect-title">{{.ToolCount}} available tools</div>
  <div class="tool-grid">
    <div class="tool-card"><strong>list_content</strong><span>Browse poems and essays</span></div>
    <div class="tool-card"><strong>read_content</strong><span>Read any public piece</span></div>
    <div class="tool-card"><strong>list_blobs</strong><span>Browse typed data artifacts</span></div>
    <div class="tool-card"><strong>read_blob</strong><span>Read image, vector, contact, dataset</span></div>
    <div class="tool-card"><strong>verify_content</strong><span>Verify Ed25519 signature</span></div>
    <div class="tool-card"><strong>request_access</strong><span>Get gate details</span></div>
    <div class="tool-card"><strong>submit_answer</strong><span>Unlock challenge-gated piece</span></div>
    <div class="tool-card"><strong>leave_comment</strong><span>React to a piece</span></div>
    <div class="tool-card"><strong>leave_message</strong><span>Send a direct note</span></div>
    <div class="tool-card"><strong>get_author_profile</strong><span>Who is {{.Author}}</span></div>
  </div>
</div>
<div class="connect-section">
  <div class="connect-title">Run your own humanMCP</div>
  <p style="font-size:.85rem;color:var(--muted);margin-bottom:.75rem;">Fork the project and publish your own content on your own terms.</p>
  <a href="https://github.com/kapoost/humanmcp-go" target="_blank" style="display:inline-block;padding:.4rem 1rem;border:1px solid var(--border);border-radius:4px;font-size:.85rem;color:var(--fg);">View on GitHub</a>
</div>
{{template "footer" .}}
</div>
</body></html>
{{end}}

{{define "css"}}
:root{--bg:#fdfcfa;--fg:#1a1a1a;--muted:#6b6b6b;--border:#e2e0db;--accent:#2a6496;--accent-light:#e8f1f8;--locked:#7a5c00;--locked-bg:#fef9ec;--tag-bg:#f0ede8;--tag-fg:#555;--max:660px;--serif:Georgia,'Times New Roman',serif;--sans:-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;}
@media(prefers-color-scheme:dark){:root{--bg:#141412;--fg:#e8e6e1;--muted:#888;--border:#2e2c28;--accent:#6baed6;--accent-light:#1a2a36;--locked:#d4a017;--locked-bg:#1e1800;--tag-bg:#252320;--tag-fg:#aaa;}}
*{box-sizing:border-box;margin:0;padding:0;}
body{background:var(--bg);color:var(--fg);font-family:var(--sans);font-size:16px;line-height:1.6;}
a{color:var(--accent);text-decoration:none;}
a:hover{text-decoration:underline;}
.container{max-width:var(--max);margin:0 auto;padding:0 1.25rem;}
.pieces{list-style:none;}
.piece-item{padding:1.1rem 0;border-bottom:1px solid var(--border);}
.piece-item:last-child{border-bottom:none;}
.piece-meta{font-size:.78rem;color:var(--muted);margin-bottom:.25rem;display:flex;gap:.6rem;align-items:center;flex-wrap:wrap;}
.type-badge{font-size:.68rem;text-transform:uppercase;letter-spacing:.05em;background:var(--tag-bg);color:var(--tag-fg);padding:1px 5px;border-radius:3px;}
.signed-badge{font-size:.65rem;background:#e8f5e9;color:#2e7d32;padding:1px 5px;border-radius:3px;border:1px solid #4caf50;}
.ots-badge{font-size:.65rem;padding:1px 5px;border-radius:3px;}
.ots-anchored{background:#e8f0fe;color:#1a3a8a;border:1px solid #6488d0;}
.ots-pending{background:#fef9e8;color:#7a5c00;border:1px solid #c9a96e;}
@media(prefers-color-scheme:dark){.signed-badge{background:#0d2b0d;color:#6abf6a;border-color:#2d6b2d;}.ots-anchored{background:#0d1229;color:#8899e0;border-color:#2d3d8a;}.ots-pending{background:#1e1800;color:#d4a017;border-color:#7a5c00;}}
.st-active{color:#2e7d32;}.st-anchored{color:#1a3a8a;}.st-pending{color:#7a5c00;}.st-none{color:var(--muted);}
@media(prefers-color-scheme:dark){.st-active{color:#6abf6a;}.st-anchored{color:#8899e0;}.st-pending{color:#d4a017;}}
.locked-badge{font-size:.68rem;background:var(--locked-bg);color:var(--locked);padding:1px 5px;border-radius:3px;border:1px solid var(--locked);}
.piece-title{font-size:1.05rem;font-weight:500;margin-bottom:.2rem;}
.piece-title a{color:var(--fg);}
.piece-title a:hover{color:var(--accent);text-decoration:none;}
.piece-desc{font-size:.88rem;color:var(--muted);}
.tags{display:flex;gap:.35rem;flex-wrap:wrap;margin-top:.35rem;}
.tag{font-size:.72rem;color:var(--muted);background:var(--tag-bg);padding:1px 6px;border-radius:10px;}
.empty{color:var(--muted);padding:2rem 0;text-align:center;}
.owner-bar{display:flex;gap:.5rem;align-items:center;margin-bottom:1.5rem;padding:.6rem .8rem;background:var(--accent-light);border:1px solid var(--accent);border-radius:6px;flex-wrap:wrap;}
.btn{display:inline-block;padding:.35rem .8rem;border-radius:4px;font-size:.82rem;cursor:pointer;border:1px solid var(--border);background:var(--bg);color:var(--fg);}
.btn:hover{background:var(--accent-light);border-color:var(--accent);color:var(--accent);}
.btn-primary{background:var(--accent);color:#fff;border-color:var(--accent);}
.btn-primary:hover{opacity:.9;background:var(--accent);color:#fff;}
.btn-sm{padding:.25rem .6rem;font-size:.78rem;}
.edit-btn{font-size:.7rem;margin-left:.4rem;padding:1px 5px;cursor:pointer;border:1px solid var(--border);border-radius:3px;background:var(--bg);color:var(--muted);}
.edit-btn:hover{border-color:var(--accent);color:var(--accent);}
{{end}}

{{define "header"}}
<header style="border-bottom:1px solid var(--border);padding:1.25rem 0 .9rem;margin-bottom:1.75rem;">
  <div style="display:flex;justify-content:space-between;align-items:flex-start;flex-wrap:wrap;gap:.4rem;">
    <div>
      <div style="font-size:1.15rem;font-weight:600;display:flex;align-items:center;gap:.5rem;">
        <a href="/" style="color:var(--fg);">{{.Author}}</a>
        <span style="font-size:.68rem;background:var(--accent-light);color:var(--accent);padding:2px 6px;border-radius:3px;border:1px solid var(--accent);">humanMCP</span>
      </div>
      {{if .Bio}}<div style="font-size:.82rem;color:var(--muted);margin-top:.2rem;">{{.Bio}}</div>{{end}}
    </div>
    <nav style="font-size:.8rem;color:var(--muted);display:flex;gap:.9rem;align-items:center;padding-top:.15rem;">
      {{if .IsOwner}}
        <a href="/llms-edit" style="color:var(--muted);" title="Edit your llms.txt agent preferences">llms.txt</a>
        <a href="/dashboard" style="color:var(--muted);">dashboard</a>
        <a href="/logout" style="color:var(--muted);">logout</a>
      {{else}}
        <a href="/images" style="color:var(--muted);">images</a>
        <a href="/contact" style="color:var(--muted);">contact</a>
        <a href="/connect" style="color:var(--accent);font-weight:500;">+ connect</a>
      {{end}}
    </nav>
  </div>
</header>
{{end}}

{{define "header-simple"}}
<header style="border-bottom:1px solid var(--border);padding:1rem 0 .75rem;margin-bottom:1.5rem;">
  <div style="font-size:1rem;font-weight:600;"><a href="/" style="color:var(--fg);">{{.Author}}</a></div>
</header>
{{end}}

{{define "footer"}}
<footer style="border-top:1px solid var(--border);margin-top:3.5rem;padding:1.25rem 0;font-size:.75rem;color:var(--muted);display:flex;justify-content:space-between;flex-wrap:wrap;gap:.5rem;">
  <span><a href="/connect" style="color:var(--muted);">connect MCP</a> &middot; <a href="https://github.com/kapoost/humanmcp-go" target="_blank" style="color:var(--muted);">github</a></span>
  <span>humanMCP v0.1 &middot; {{.Author}}</span>
</footer>
{{end}}


{{define "new.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{if .Piece}}Edit — {{.Piece.Slug}}{{else}}New post{{end}} — {{.Author}}</title>
<style>{{template "css" .}}
.compose{max-width:560px;margin:0 auto;}
textarea{width:100%;padding:.7rem .8rem;border:1.5px solid var(--border);border-radius:8px;background:var(--bg);color:var(--fg);font-size:1rem;line-height:1.7;resize:vertical;font-family:inherit;}
textarea:focus{outline:none;border-color:var(--accent);}
.field{margin-bottom:.7rem;}
.fl{font-size:.75rem;color:var(--muted);display:block;margin-bottom:.25rem;}
input[type=text],input[type=datetime-local],select{width:100%;padding:.45rem .6rem;border:1px solid var(--border);border-radius:4px;background:var(--bg);color:var(--fg);font-size:.88rem;}
input:focus,select:focus{outline:none;border-color:var(--accent);}
.row2{display:grid;grid-template-columns:1fr 1fr;gap:.6rem;margin-bottom:.7rem;}
.radio-group{display:flex;gap:.6rem;flex-wrap:wrap;margin-bottom:.7rem;}
.radio-group label{font-size:.88rem;color:var(--fg);cursor:pointer;display:flex;align-items:center;gap:.3rem;font-weight:normal;}
.radio-group input[type=radio]{width:auto;margin:0;}
details{border:1px solid var(--border);border-radius:6px;padding:.65rem .8rem;margin-bottom:.7rem;}
details summary{font-size:.78rem;color:var(--muted);cursor:pointer;user-select:none;list-style:none;display:flex;align-items:center;gap:.4rem;}
details summary::-webkit-details-marker{display:none;}
details summary::before{content:"⊕";color:var(--accent);}
details[open] summary::before{content:"⊖";}
details > *:not(summary){margin-top:.65rem;}
.file-area{border:2px dashed var(--border);border-radius:6px;padding:1.1rem;text-align:center;cursor:pointer;font-size:.85rem;color:var(--muted);margin-bottom:.7rem;transition:border-color .15s;}
.file-area:hover,.file-area.drag{border-color:var(--accent);color:var(--accent);}
.file-area input[type=file]{display:none;}
.file-name{margin-top:.3rem;font-weight:500;color:var(--fg);font-size:.82rem;}
.type-grid{display:flex;gap:.35rem;flex-wrap:wrap;}
.type-label{font-size:.78rem;cursor:pointer;padding:.2rem .55rem;border:1px solid var(--border);border-radius:10px;color:var(--muted);display:inline-flex;align-items:center;gap:.25rem;}
input[type=radio]:checked + .type-label{border-color:var(--accent);background:var(--accent-light);color:var(--accent);}
.type-opt{display:contents;}
</style>
</head>
<body>
<div class="container">
{{template "header-simple" .}}

<div class="compose">
<div style="display:flex;justify-content:space-between;align-items:baseline;margin-bottom:1rem;">
  <h1 style="font-size:.95rem;font-weight:500;color:var(--muted);">
    {{if .Piece}}Editing: {{.Piece.Slug}}{{else}}New post{{end}}
  </h1>
  <a href="{{if .Piece}}/p/{{.Piece.Slug}}{{else}}/{{end}}" style="font-size:.82rem;color:var(--muted);">cancel</a>
</div>

<form method="POST" enctype="multipart/form-data">
{{if .Piece}}<input type="hidden" name="slug_override" value="{{.Piece.Slug}}">{{end}}

<div class="field">
  <textarea name="body" rows="9" placeholder="What do you want to share?">{{if .Piece}}{{.Piece.Body}}{{end}}</textarea>
</div>

<div class="field">
  <label class="fl">Title <span style="opacity:.5">(optional)</span></label>
  <input type="text" name="title" value="{{if .Piece}}{{.Piece.Title}}{{end}}" placeholder="Auto-generated from first line if empty">
</div>

<div class="file-area" id="drop-zone" onclick="this.querySelector('input[type=file]').click()">
  <input type="file" name="file">
  &#8679; attach a file &mdash; image, PDF, CSV, anything
  <div class="file-name" id="file-name"></div>
</div>

<div class="field">
  <label class="fl">Who can read this?</label>
  <div class="radio-group">
    <label><input type="radio" name="access" value="public" {{if not .Piece}}checked{{else if eq (print .Piece.Access) "public"}}checked{{end}}> &#127760; everyone</label>
    <label><input type="radio" name="access" value="locked" {{if .Piece}}{{if eq (print .Piece.Access) "locked"}}checked{{end}}{{end}}> &#128274; locked</label>
    <label><input type="radio" name="access" value="members" {{if .Piece}}{{if eq (print .Piece.Access) "members"}}checked{{end}}{{end}}> &#128100; members</label>
  </div>
</div>

<details {{if .Piece}}open{{end}}>
  <summary>more options</summary>

  <div class="field">
    <label class="fl">Content type</label>
    <div class="type-grid">
      {{range (slice "note" "poem" "essay" "image" "contact" "dataset" "vector" "document" "capsule")}}
      <span class="type-opt">
        <input type="radio" name="type" value="{{.}}" id="type_{{.}}" style="display:none;"
          {{if $.Piece}}{{if eq $.Piece.Type .}}checked{{end}}{{else}}{{if eq . "note"}}checked{{end}}{{end}}>
        <label for="type_{{.}}" class="type-label">{{.}}</label>
      </span>
      {{end}}
    </div>
  </div>
  <div class="row2">
    <div>
      <label class="fl">Gate type <span style="opacity:.5">(when locked)</span></label>
      <select name="gate">
        <option value="challenge" {{if .Piece}}{{if eq (print .Piece.Gate) "challenge"}}selected{{end}}{{end}}>Question</option>
        <option value="manual"    {{if .Piece}}{{if eq (print .Piece.Gate) "manual"}}selected{{end}}{{end}}>Manual approval</option>
        <option value="time"      {{if .Piece}}{{if eq (print .Piece.Gate) "time"}}selected{{end}}{{end}}>Time lock</option>
        <option value="trade"     {{if .Piece}}{{if eq (print .Piece.Gate) "trade"}}selected{{end}}{{end}}>Trade</option>
      </select>
    </div>
    <div>
      <label class="fl">Unlock after <span style="opacity:.5">(time gate)</span></label>
      <input type="datetime-local" name="unlock_after" value="{{if .Piece}}{{isoDate .Piece.UnlockAfter}}{{end}}">
    </div>
  </div>

  <div class="row2">
    <div><label class="fl">Challenge question</label><input type="text" name="challenge" value="{{if .Piece}}{{.Piece.Challenge}}{{end}}" placeholder="What do we call each other?"></div>
    <div><label class="fl">Answer</label><input type="text" name="answer" value="{{if .Piece}}{{.Piece.Answer}}{{end}}" placeholder="answer"></div>
  </div>

</details>

<input type="hidden" name="do_sign" id="do_sign_field" value="0">
<div style="display:flex;gap:.6rem;align-items:center;margin-top:.5rem;flex-wrap:wrap;">
  <button type="submit" class="btn btn-primary" style="padding:.4rem 1.2rem;" onclick="document.getElementById('do_sign_field').value='0'">
    {{if .Piece}}Save{{else}}Post{{end}}
  </button>
  <button type="submit" class="btn" style="padding:.4rem 1.2rem;border-color:var(--accent);color:var(--accent);" onclick="document.getElementById('do_sign_field').value='1'" title="Save and apply Ed25519 signature">
    {{if .Piece}}Save &amp; Sign{{else}}Post &amp; Sign{{end}} &#10003;
  </button>
  {{if .Piece}}{{with .Piece}}{{if .Signature}}
  <span style="font-size:.72rem;color:var(--muted);margin-left:.2rem;">currently signed</span>
  {{else}}
  <span style="font-size:.72rem;color:#c0392b;margin-left:.2rem;">unsigned</span>
  {{end}}{{end}}{{end}}
</div>

</form>
</div>

{{template "footer" .}}
</div>
{{if .AIMetadata}}
<div id="ai-panel" style="display:none;border:1px solid var(--border);border-radius:8px;padding:1rem;margin-bottom:.7rem;background:var(--card);">
  <div style="display:flex;align-items:center;gap:.6rem;margin-bottom:.75rem;">
    <span style="font-size:.78rem;font-weight:500;color:var(--muted);text-transform:uppercase;letter-spacing:.06em;">AI metadata assist</span>
    <span id="ai-status" style="font-size:.78rem;color:var(--muted);"></span>
  </div>
  <div class="field">
    <label class="fl">Your Anthropic API key <span style="opacity:.5">(used once, not stored)</span></label>
    <input type="text" id="ai-key" placeholder="sk-ant-..." style="font-family:monospace;font-size:.82rem;">
  </div>
  <button type="button" id="ai-btn" class="btn" style="padding:.35rem .9rem;font-size:.82rem;" onclick="runAI()">Generate metadata</button>
</div>
{{end}}
<script>
(function(){
  var dz=document.getElementById('drop-zone');
  var fi=dz.querySelector('input[type=file]');
  var fn=document.getElementById('file-name');

  function onFile(f){
    fn.textContent=f.name;
    {{if .AIMetadata}}
    var panel=document.getElementById('ai-panel');
    if(panel && f.type.startsWith('image/')){
      panel.style.display='block';
      document.getElementById('ai-status').textContent='Image ready — click Generate';
    }
    {{end}}
  }

  fi.onchange=function(){if(fi.files[0])onFile(fi.files[0]);};
  dz.addEventListener('dragover',function(e){e.preventDefault();dz.classList.add('drag');});
  dz.addEventListener('dragleave',function(){dz.classList.remove('drag');});
  dz.addEventListener('drop',function(e){
    e.preventDefault();dz.classList.remove('drag');
    var f=e.dataTransfer.files[0];if(!f)return;
    var dt=new DataTransfer();dt.items.add(f);fi.files=dt.files;onFile(f);
  });
})();

{{if .AIMetadata}}
async function runAI(){
  var key=document.getElementById('ai-key').value.trim();
  if(!key){alert('Enter your Anthropic API key');return;}
  var fi=document.querySelector('input[type=file]');
  if(!fi.files[0]){alert('Select an image first');return;}

  var status=document.getElementById('ai-status');
  var btn=document.getElementById('ai-btn');
  btn.disabled=true;
  status.textContent='Reading image…';

  // Read image as base64
  var b64=await new Promise(function(res,rej){
    var r=new FileReader();
    r.onload=function(){res(r.result.split(',')[1]);};
    r.onerror=rej;
    r.readAsDataURL(fi.files[0]);
  });
  var mime=fi.files[0].type||'image/jpeg';

  status.textContent='Asking Claude…';
  try {
    var resp=await fetch('https://api.anthropic.com/v1/messages',{
      method:'POST',
      headers:{
        'Content-Type':'application/json',
        'x-api-key':key,
        'anthropic-version':'2023-06-01',
        'anthropic-dangerous-direct-browser-access':'true'
      },
      body:JSON.stringify({
        model:'claude-sonnet-4-20250514',
        max_tokens:500,
        messages:[{
          role:'user',
          content:[
            {type:'image',source:{type:'base64',media_type:mime,data:b64}},
            {type:'text',text:'Analyse this image and return ONLY valid JSON with these fields:
{
  "title": "short human title (3-6 words)",
  "slug": "url-safe-slug-with-dashes",
  "description": "one sentence for humans, evocative and honest",
  "description_agents": "one sentence for AI agents: precise visual description, object list, colors, composition",
  "tags": "comma-separated tags (3-6 tags, lowercase)"
}
No markdown, no explanation, just JSON.'}
          ]
        }]
      })
    });
    var data=await resp.json();
    var text=data.content[0].text.trim();
    // Strip markdown fences if present
    if(text.indexOf('json')===3){text=text.slice(text.indexOf('\n')+1);}
    if(text.lastIndexOf('\n')>0){text=text.slice(0,text.lastIndexOf('\n'));}
    text=text.trim();
    var meta=JSON.parse(text);

    // Fill form fields
    var q=function(n){return document.querySelector('[name="'+n+'"]');};
    if(meta.title && q('title')) q('title').value=meta.title;
    if(meta.slug && q('slug')) q('slug').value=meta.slug;
    if(meta.description && q('description')) q('description').value=meta.description;
    if(meta.tags && q('tags')) q('tags').value=meta.tags;
    // Set type to image
    var imgRadio=document.getElementById('type_image');
    if(imgRadio){imgRadio.checked=true;}
    // Open details
    var det=document.querySelector('details');
    if(det)det.open=true;

    status.textContent='✓ Done — review and edit below';
    status.style.color='var(--accent)';

    // Show agent description as a hint
    if(meta.description_agents){
      var hint=document.createElement('p');
      hint.style.cssText='font-size:.75rem;color:var(--muted);margin:.5rem 0 0;font-style:italic;';
      hint.textContent='Agent description: '+meta.description_agents;
      document.getElementById('ai-panel').appendChild(hint);
      // Also store it — forker can add an about/agent_desc field later
      var hidden=document.createElement('input');
      hidden.type='hidden';hidden.name='agent_description';hidden.value=meta.description_agents;
      document.getElementById('ai-panel').appendChild(hidden);
    }
  } catch(e){
    status.textContent='Error: '+e.message;
    status.style.color='#c0392b';
  }
  btn.disabled=false;
}
{{end}}
</script>
</body></html>
{{end}}


{{define "images.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Images — {{.Author}}</title>
<style>{{template "css" .}}
.img-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:1rem;margin-top:1.5rem;}
.img-card{border:1px solid var(--border);border-radius:8px;overflow:hidden;background:var(--tag-bg);}
.img-card img{width:100%;height:180px;object-fit:cover;display:block;}
.img-info{padding:.6rem .75rem;}
.img-title{font-size:.88rem;font-weight:500;color:var(--fg);margin-bottom:.2rem;}
.img-desc{font-size:.76rem;color:var(--muted);line-height:1.4;}
.img-tags{margin-top:.35rem;display:flex;gap:.3rem;flex-wrap:wrap;}
</style>
</head>
<body>
<div class="container">
{{template "header-simple" .}}
<a href="/" style="font-size:.85rem;color:var(--muted);display:inline-block;margin-bottom:1.5rem;">&#8592; back</a>
<h1 style="font-size:1.1rem;font-weight:500;">Images</h1>
{{if .Images}}
<div class="img-grid">
{{range .Images}}
<div class="img-card">
  {{if .FileRef}}<a href="/{{.FileRef}}"><img src="/{{.FileRef}}" alt="{{.Title}}" loading="lazy"></a>{{end}}
  <div class="img-info">
    <div class="img-title">{{if .Title}}{{.Title}}{{else}}{{.Slug}}{{end}}</div>
    {{if .Description}}<div class="img-desc">{{.Description}}</div>{{end}}
    {{if .Tags}}<div class="img-tags">{{range .Tags}}<span class="tag">#{{.}}</span>{{end}}</div>{{end}}
  </div>
</div>
{{end}}
</div>
{{else}}
<div class="empty" style="margin-top:2rem;">No images yet.</div>
{{end}}
{{template "footer" .}}
</div>
</body></html>
{{end}}
{{define "llms-edit.html"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>llms.txt editor — {{.Author}}</title>
<style>{{template "css" .}}
.llms-wrap{max-width:600px;margin:0 auto;}
textarea{width:100%;padding:.75rem .9rem;border:1.5px solid var(--border);border-radius:8px;background:var(--bg);color:var(--fg);font-family:monospace;font-size:.88rem;line-height:1.75;resize:vertical;}
textarea:focus{outline:none;border-color:var(--accent);}
.sig-box{background:var(--accent-light);border:1px solid var(--accent);border-radius:6px;padding:.75rem 1rem;font-family:monospace;font-size:.72rem;color:var(--accent);word-break:break-all;margin-top:1rem;}
.sig-label{font-size:.68rem;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);margin-bottom:.3rem;font-family:var(--sans);}
.tip-box{background:var(--tag-bg);border:1px solid var(--border);border-radius:6px;padding:.8rem 1rem;margin-bottom:1.25rem;}
.tip-box p{font-size:.8rem;color:var(--muted);line-height:1.6;margin:0;}
.tip-box code{font-size:.78rem;background:var(--bg);padding:1px 5px;border-radius:3px;border:1px solid var(--border);}
.section-title{font-size:.7rem;font-weight:500;color:var(--muted);text-transform:uppercase;letter-spacing:.07em;margin-bottom:.5rem;}
.btn-row{display:flex;gap:.6rem;align-items:center;margin-top:.75rem;flex-wrap:wrap;}
.starter-btn{font-size:.75rem;padding:.25rem .65rem;border-radius:4px;border:1px solid var(--border);background:var(--bg);color:var(--muted);cursor:pointer;}
.starter-btn:hover{border-color:var(--accent);color:var(--accent);}
.public-url{font-family:monospace;font-size:.82rem;background:var(--tag-bg);padding:.45rem .7rem;border-radius:4px;border:1px solid var(--border);color:var(--fg);display:inline-block;margin-bottom:1.25rem;word-break:break-all;}
</style>
</head>
<body>
<div class="container">
{{template "header-simple" .}}
<div class="llms-wrap">

<div style="display:flex;justify-content:space-between;align-items:baseline;margin-bottom:.6rem;">
  <h1 style="font-size:1rem;font-weight:500;">llms.txt — agent preferences</h1>
  <a href="/" style="font-size:.82rem;color:var(--muted);">&#8592; back</a>
</div>

<div class="public-url">&#127760; https://{{.Domain}}/llms.txt</div>

<div class="tip-box">
  <p>This file is served publicly at <code>/llms.txt</code> and signed with your Ed25519 key. Point any AI agent here to share your preferences without repeating yourself. Agents can verify the signature via <code>verify_content llms-txt</code> on your MCP server.</p>
</div>

<form method="POST" action="/llms-edit">
<div style="margin-bottom:.5rem;">
  <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:.35rem;">
    <span class="section-title">content</span>
    <button type="button" class="starter-btn" onclick="loadStarter()">load starter template</button>
  </div>
  <textarea name="body" id="llms-body" rows="28" placeholder="# Your name&#10;&#10;&gt; One-line summary for agents.&#10;&#10;## Instructions&#10;&#10;- ...">{{.Body}}</textarea>
  <div style="font-size:.7rem;color:var(--muted);text-align:right;margin-top:2px;" id="char-count"></div>
</div>

<div class="btn-row">
  <button type="submit" class="btn btn-primary" style="padding:.4rem 1.2rem;">Save &amp; sign</button>
  <a href="/llms.txt" style="font-size:.8rem;color:var(--muted);" target="_blank">preview ↗</a>
  {{if .Signature}}<span style="font-size:.72rem;color:#2e7d32;">&#10003; currently signed</span>{{end}}
</div>
</form>

{{if .Signature}}
<div style="margin-top:1.5rem;">
  <div class="sig-label">Ed25519 signature (latest save)</div>
  <div class="sig-box">{{.Signature}}</div>
  <p style="font-size:.74rem;color:var(--muted);margin-top:.5rem;">Agents can verify this via your MCP server: <code>verify_content llms-txt</code></p>
</div>
{{end}}

<div style="margin-top:2rem;padding-top:1rem;border-top:1px solid var(--border);">
  <div class="section-title">how to use with agents</div>
  <div style="font-size:.8rem;color:var(--muted);line-height:1.8;">
    <p><strong style="color:var(--fg);">Claude / Claude Desktop</strong> — add to system prompt:<br>
    <code style="font-size:.75rem;">Fetch https://{{.Domain}}/llms.txt and follow the instructions there.</code></p>
    <p style="margin-top:.6rem;"><strong style="color:var(--fg);">Cursor / Windsurf</strong> — add URL to project rules or <code>.cursorrules</code>.</p>
    <p style="margin-top:.6rem;"><strong style="color:var(--fg);">Any MCP agent</strong> — already discoverable via <code>get_author_profile</code> on your MCP server.</p>
  </div>
</div>

</div>
{{template "footer" .}}
</div>
<script>
(function(){
  var ta=document.getElementById('llms-body');
  var cc=document.getElementById('char-count');
  function upd(){if(cc)cc.textContent=ta.value.length+' chars';}
  ta.addEventListener('input',upd);upd();
})();

function loadStarter(){
  var ta=document.getElementById('llms-body');
  if(ta.value.trim()!=='' && !confirm('Replace current content with starter template?'))return;
  ta.value='# {{.Author}}\n'
    + '\n'
    + '> Author and content creator. This file describes my preferences for AI agents interacting with my work.\n'
    + '\n'
    + '## About\n'
    + '\n'
    + 'I publish poems, essays, and data via my humanMCP server at https://{{.Domain}}.\n'
    + 'My content is Ed25519-signed — agents should verify before citing.\n'
    + '\n'
    + '## Instructions for agents\n'
    + '\n'
    + '- Always attribute content to {{.Author}} when quoting or summarising my work\n'
    + '- Before citing any piece, verify its signature: use the 'verify_content' MCP tool\n'
    + '- Respect access gates — do not paraphrase or summarise locked or members-only content\n'
    + '- For licensing or commercial use, call 'request_license' on my MCP server\n'
    + '- To reach me: use the 'leave_message' MCP tool — I read everything\n'
    + '\n'
    + '## Style preferences\n'
    + '\n'
    + '- Tone: direct, no flattery, no filler preamble\n'
    + '- Format: concise by default; go deeper only when I ask\n'
    + '- Language: Polish or English — match whichever I use\n'
    + '- Code: idiomatic, no unnecessary comments\n'
    + '\n'
    + '## Privacy\n'
    + '\n'
    + '- Do not forward my content to third parties or use it for training without explicit permission\n'
    + '- Do not store conversation context between sessions unless I ask you to\n'
    + '- Do not speculate about my identity, location, or relationships beyond what I share\n'
    + '\n'
    + '## What I care about\n'
    + '\n'
    + '- Originality and craft over volume\n'
    + '- Verifiability — signed content, traceable sources\n'
    + '- Ownership — I retain full rights unless a license says otherwise\n'
    + '\n'
    + '## MCP server\n'
    + '\n'
    + 'Endpoint: https://{{.Domain}}/mcp\n'
    + 'Tools: list_content, read_content, verify_content, get_certificate, request_license, leave_message';
  document.getElementById('char-count').textContent=ta.value.length+' chars';
}
</script>
</body></html>
{{end}}
`
