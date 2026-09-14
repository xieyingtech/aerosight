from pathlib import Path
from PIL import Image,ImageDraw,ImageFont
import json,subprocess,re,bisect,math
ROOT=Path(__file__).resolve().parents[3];BASE=ROOT/'.build/msup-demo';REC=BASE/'recording';OUT=BASE/'deliverables';EDIT=REC/'edit-v2';EDIT.mkdir(exist_ok=True)
FONT='/System/Library/Fonts/Hiragino Sans GB.ttc'
def font(n):return ImageFont.truetype(FONT,n)
F=font(34);H=font(28);S=font(22)
def duration(p):return float(subprocess.check_output(['ffprobe','-v','error','-show_entries','format=duration','-of','default=nw=1:nk=1',str(p)]))
def run(cmd):subprocess.run(cmd,check=True,stdout=subprocess.DEVNULL,stderr=subprocess.PIPE)
segments=json.loads((ROOT/'docs/demo/narration.json').read_text());raw=json.loads((REC/'take3/frames.json').read_text())
# CDP screencast places the 1280x720 CSS viewport at the origin of a 1920 canvas.
# Crop the unused canvas and enlarge to match the requested 1.5x presentation size.
scene_ids=[None,[0,1,2], [5,6,8,12,13,14,15,16,17,18,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34], [39,40,42,47], [50,52,54], [55,56,58,60,62,63,65,66,67,68], [72,74,76,77,81,82], [87,89,91,92,93,95,96,97,99,100], None]
thumbs={}
def browser(n):
 if n not in thumbs:thumbs[n]=Image.open(REC/f'take3/{n:06d}.jpg').convert('RGB').crop((0,0,1280,720)).resize((1920,1080),Image.Resampling.LANCZOS)
 return thumbs[n]
def card(end=False):
 return Image.open(OUT/'AeroSight封面.png').convert('RGB').resize((1920,1080),Image.Resampling.LANCZOS)
allsubs=[];timeline=[];concat=[];offset=0.;seq=0;audiofiles=[]
for i,s in enumerate(segments):
 audio=REC/f'timed/{i:02d}.mp3';ad=duration(audio);total=math.ceil((ad+1.6)*25)/25;s.update(start=offset,duration=total,audioDuration=ad)
 wav=EDIT/f'audio-{i}.wav';run(['ffmpeg','-y','-i',str(audio),'-af','adelay=600,apad','-t',str(total),'-ar','48000','-ac','1',str(wav)]);audiofiles.append(wav)
 items=[w for sub in json.loads((REC/f'timed/{i:02d}.json').read_text()) for w in sub['items']]
 subs=[];buf=[]
 for w in items:
  buf.append(w);text=''.join(x['text'] for x in buf).strip()
  if (re.search('[，。；！？]$',text) and len(text)>=9) or len(text)>=30:
   subs.append({'text':text,'start':buf[0]['start_time']/1000+.6,'end':buf[-1]['end_time']/1000+.6});buf=[]
 if buf:subs.append({'text':''.join(x['text'] for x in buf).strip(),'start':buf[0]['start_time']/1000+.6,'end':buf[-1]['end_time']/1000+.6})
 for j,x in enumerate(subs):
  x['end']=min(total,x['end']+.12,subs[j+1]['start'] if j+1<len(subs) else total)
  assert x['end']>x['start'] and x['end']<=total
  allsubs.append(dict(text=x['text'],start=x['start']+offset,end=x['end']+offset))
 rng=scene_ids[i];clips=[]
 if rng:
  ids=rng
  weights=[max(.08,min(3.,raw[ids[j+1]]['t']-raw[n]['t'])) if j+1<len(ids) else 5. for j,n in enumerate(ids)]
  cumulative=0
  for n,w in zip(ids,weights):clips.append((cumulative/sum(weights)*total,n));cumulative+=w
 else:clips=[(0,None)]
 bounds=sorted(set([0.,total]+[t for t,n in clips]+[x[k] for x in subs for k in ['start','end']]))
 for a,b in zip(bounds,bounds[1:]):
  if b-a<.0001:continue
  n=clips[max(0,bisect.bisect_right([x[0] for x in clips],a)-1)][1]
  if n is None:im=card(i==8)
  else:
   im=Image.new('RGB',(1920,1080),'#0b1424');im.paste(browser(n),(0,0))
  d=ImageDraw.Draw(im)
  if n is not None:
   label=s['title'];w=d.textlength(label,font=H)
   d.rounded_rectangle((1880-w-34,18,1900,65),radius=10,fill='#0b1424')
   d.text((1880-w-17,25),label,font=H,fill='white')
  d.rectangle((0,984,1920,1080),fill='#0b1424')
  sub=next((x['text'] for x in subs if x['start']<=a+.001<x['end']), '')
  if sub:
   width=d.textlength(sub,font=F);d.text(((1920-width)/2,1007),sub,font=F,fill='white')
  d.rectangle((0,1076,int(1920*(i+1)/9),1079),fill='#64b8ff')
  p=EDIT/f'{seq:05d}.png';im.save(p);concat.append(f"file '{p}'\nduration {b-a:.6f}\n");timeline.append({'file':p.name,'start':offset+a,'end':offset+b,'sourceFrame':n,'segment':i,'subtitle':sub});seq+=1
 offset+=total
 print('segment',i,'duration',round(total,2),'frames',len(clips),flush=True)
concat.append(f"file '{p}'\n");(EDIT/'video.ffconcat').write_text('ffconcat version 1.0\n'+''.join(concat))
(EDIT/'audio.ffconcat').write_text('ffconcat version 1.0\n'+''.join(f"file '{x}'\n" for x in audiofiles))
def tc(t):
 ms=round(t*1000);return f'{ms//3600000:02d}:{ms//60000%60:02d}:{ms//1000%60:02d},{ms%1000:03d}'
(OUT/'AeroSight演示字幕.srt').write_text('\n\n'.join(f'{j+1}\n{tc(x["start"])} --> {tc(x["end"])}\n{x["text"]}' for j,x in enumerate(allsubs))+'\n')
(EDIT/'timeline.json').write_text(json.dumps(timeline,ensure_ascii=False,indent=2));(OUT/'制作清单.json').write_text(json.dumps({'resolution':'1920x1080','fps':25,'duration':offset,'recording':'CDP Page.screencastFrame through CUA browser automation; edited completed Run 3 replay','ttsEndpoint':'https://api.stepfun.com/step_plan/v1/audio/speech','ttsModel':'stepaudio-2.5-tts','voice':'cixingnansheng','subtitleTiming':'StepFun word timestamps','segments':segments},ensure_ascii=False,indent=2))
run(['ffmpeg','-y','-f','concat','-safe','0','-i',str(EDIT/'audio.ffconcat'),'-af','loudnorm=I=-16:TP=-1.5:LRA=11','-c:a','libmp3lame','-b:a','192k',str(OUT/'AeroSight解说配音.mp3')])
run(['ffmpeg','-y','-f','concat','-safe','0','-i',str(EDIT/'video.ffconcat'),'-i',str(OUT/'AeroSight解说配音.mp3'),'-t',str(offset),'-vf','fps=25,format=yuv420p','-c:v','libx264','-preset','fast','-crf','18','-c:a','aac','-b:a','192k','-movflags','+faststart',str(OUT/'AeroSight演示视频1080p.mp4')])
print('COMPLETE',offset,seq)
