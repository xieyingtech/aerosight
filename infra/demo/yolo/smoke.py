"""Exercise the live demo API with downloaded RGB assets and negative cases."""
import hashlib
import json
from pathlib import Path
from uuid import uuid4

import httpx
from PIL import Image, ImageDraw

root = Path(__file__).resolve().parents[3]
media = root / '.build/msup-demo/source-media/rgb/images'
out = root / '.build/msup-demo/yolo-results'
out.mkdir(parents=True, exist_ok=True)
with httpx.Client(base_url='http://127.0.0.1:8091', timeout=180) as client:
    health = client.get('/healthz')
    health.raise_for_status()
    print(health.json(), flush=True)
    summary = []
    for path in sorted(media.glob('*.jpeg')):
        payload = {'schemaVersion': 'aerosight.algorithm.input/v1',
                   'runId': str(uuid4()), 'projectId': 1,
                   'definition': {'providerType': 'http-json', 'modelOrProcess': 'yolo11n',
                                  'executionMode': 'synchronous', 'mappingVersion': 'v1'},
                   'inputAsset': {'checksumSha256': hashlib.sha256(path.read_bytes()).hexdigest()},
                   'parameters': {'localAsset': path.name}}
        response = client.post('/infer', json=payload)
        response.raise_for_status()
        result = response.json()
        (out / (path.stem + '.json')).write_text(json.dumps(result, indent=2))
        boxes = result['result']['detections']
        with Image.open(path) as original:
            im = original.convert('RGB')
        draw = ImageDraw.Draw(im)
        for box in boxes:
            g = box['pixelGeometry']; x, y, w, h = [g[k] for k in ('x', 'y', 'width', 'height')]
            assert 0 <= x < x+w <= im.width+0.01 and 0 <= y < y+h <= im.height+0.01
            assert 0 <= box['confidence'] <= 1
            draw.rectangle((x,y,x+w,y+h), outline='red', width=max(3, im.width//1000))
            draw.text((x,y), f"{box['label']} {box['confidence']:.2f}", fill='red',
                      font_size=max(16, im.width//240))
        im.thumbnail((1920,1920))
        im.save(out / (path.stem + '-detected.jpg'))
        counts = {}
        for box in boxes:
            counts[box['label']] = counts.get(box['label'], 0) + 1
        record = {'file': path.name, 'counts': counts, **result['metrics']}
        summary.append(record)
        print(record, flush=True)
    # Distinguish real failures from valid empty detections.
    payload['inputAsset']['checksumSha256'] = '0'*64
    assert client.post('/infer', json=payload).status_code == 422
    payload['parameters']['localAsset'] = '../../../../.env.local'
    assert client.post('/infer', json=payload).status_code == 400
    payload['parameters'] = {}
    payload['inputAsset']['accessUrl'] = 'http://127.0.0.1/private'
    assert client.post('/infer', json=payload).status_code == 400
    assert client.post('/predict', files={'file': ('bad.jpg', b'invalid', 'image/jpeg')}).status_code == 422
    sample = sorted(media.glob('*.jpeg'))[0]
    with sample.open('rb') as f:
        response = client.post('/predict', files={'file': (sample.name, f, 'image/jpeg')})
    response.raise_for_status()
    (out / 'summary.json').write_text(json.dumps({'health': health.json(), 'samples': summary,
        'checks': ['health', 'six-real-images', 'original-pixel-bounds', 'checksum-rejection',
                   'path-traversal-rejection', 'url-policy-rejection', 'invalid-image-rejection',
                   'multipart-upload']}, indent=2))
    print('All API smoke checks passed.', flush=True)
