"""Local-only YOLO demo adapter; launch with uvicorn bound to 127.0.0.1."""
import hashlib
import io
import os
import threading
import time
from contextlib import asynccontextmanager
from pathlib import Path
from urllib.parse import urlsplit

import httpx
import torch
from fastapi import FastAPI, File, HTTPException, UploadFile
from PIL import Image, UnidentifiedImageError
from pydantic import BaseModel, Field
# Keep Pillow decoding independent of Ultralytics optional HEIF fallback.
PIL_OPEN = Image.open
from ultralytics import YOLO

ROOT = Path(__file__).resolve().parents[3]
MEDIA = (ROOT / '.build/msup-demo/source-media/rgb/images').resolve()
WEIGHTS = ROOT / '.build/msup-demo/models/yolo11n.pt'
LIMIT = 40 * 1024 * 1024
DEVICE = os.getenv('YOLO_DEVICE', 'mps' if torch.backends.mps.is_available() else 'cpu')
ALLOWED_ORIGINS = set(filter(None, os.getenv('YOLO_ASSET_ORIGINS', '').split(',')))
ALLOWED_HOSTS = set(filter(None, os.getenv('YOLO_ASSET_HOSTS', '').split(',')))
lock = threading.Lock()
model = None
digest = ''


@asynccontextmanager
async def lifespan(app):
    global model, digest
    WEIGHTS.parent.mkdir(parents=True, exist_ok=True)
    model = YOLO(str(WEIGHTS))
    digest = 'sha256:' + hashlib.sha256(WEIGHTS.read_bytes()).hexdigest()
    # Fail startup if the configured device cannot actually run inference.
    model.predict(Image.new('RGB', (640, 640)), device=DEVICE, verbose=False)
    yield


app = FastAPI(title='AeroSight Local YOLO Demo', lifespan=lifespan)


class Parameters(BaseModel):
    confidence: float = Field(default=0.25, ge=0.01, le=1)
    imgsz: int = Field(default=1280, ge=320, le=2048, multiple_of=32)
    localAsset: str | None = None


class Asset(BaseModel):
    accessUrl: str = ''
    checksumSha256: str = Field(pattern=r'^[a-f0-9]{64}$')


class Input(BaseModel):
    schemaVersion: str = Field(pattern=r'^aerosight\.algorithm\.input/v1$')
    runId: str
    inputAsset: Asset
    parameters: Parameters = Field(default_factory=Parameters)


def predict(data: bytes, params: Parameters, run_id: str = ''):
    if len(data) > LIMIT:
        raise HTTPException(413, 'Image exceeds 40 MiB')
    try:
        im = PIL_OPEN(io.BytesIO(data))
        if im.format not in ('JPEG', 'MPO', 'PNG'):
            raise HTTPException(415, 'Only JPEG (including DJI MPO) and PNG supported')
        im.load()
        # Keep stored pixel orientation, matching source asset coordinates.
        im = im.convert('RGB')
    except (UnidentifiedImageError, OSError, Image.DecompressionBombError):
        raise HTTPException(422, 'Cannot decode image')
    start = time.perf_counter()
    with lock:
        result = model.predict(im, device=DEVICE, imgsz=params.imgsz,
                               conf=params.confidence, classes=[0, 1, 2, 3, 5, 7],
                               max_det=300, verbose=False)[0]
    detections = []
    for i, box in enumerate(result.boxes.cpu()):
        x1, y1, x2, y2 = box.xyxy[0].tolist()
        detections.append({
            'detectionKey': str(i), 'label': result.names[int(box.cls[0])],
            'confidence': float(box.conf[0]),
            'pixelGeometry': {'type': 'bbox', 'x': x1, 'y': y1,
                              'width': x2 - x1, 'height': y2 - y1},
            'attributes': {}})
    return {'result': {'kind': 'detection', 'detections': detections},
            'modelRevision': 'yolo11n-coco', 'modelDigest': digest,
            'inputRunId': run_id,
            'image': {'width': im.width, 'height': im.height},
            'metrics': {'inferenceMs': round((time.perf_counter()-start)*1000, 1),
                        'device': DEVICE, 'imgsz': params.imgsz}}


@app.get('/healthz')
def health():
    return {'ok': model is not None, 'model': 'yolo11n', 'device': DEVICE,
            'modelDigest': digest}


@app.post('/predict')
def upload(file: UploadFile = File(...)):
    """Quick multipart upload test; inference is real, no platform Run is created."""
    return predict(file.file.read(LIMIT + 1), Parameters())


@app.post('/infer')
def infer(payload: Input):
    if payload.parameters.localAsset:
        # Explicit demo extension: only RGB images within the fixed media root.
        path = (MEDIA / payload.parameters.localAsset).resolve()
        if not path.is_relative_to(MEDIA) or not path.is_file():
            raise HTTPException(400, 'Invalid demo asset')
        if path.stat().st_size > LIMIT:
            raise HTTPException(413, 'Image exceeds 40 MiB')
        data = path.read_bytes()
    else:
        url = urlsplit(payload.inputAsset.accessUrl)
        if (url.scheme != 'https' or url.username or url.password
                or not ((url.hostname in ALLOWED_HOSTS and url.port in (None, 443))
                        or f'{url.scheme}://{url.netloc}' in ALLOWED_ORIGINS)):
            raise HTTPException(400, 'HTTPS asset host must be in YOLO_ASSET_HOSTS')
        # Redirects disabled so the explicitly configured host boundary is retained.
        try:
            with httpx.stream('GET', payload.inputAsset.accessUrl,
                              timeout=60, follow_redirects=False) as response:
                if response.status_code != 200:
                    raise HTTPException(502, 'Asset download failed')
                data = bytearray()
                for chunk in response.iter_bytes():
                    data.extend(chunk)
                    if len(data) > LIMIT:
                        raise HTTPException(413, 'Image exceeds 40 MiB')
                data = bytes(data)
        except httpx.HTTPError:
            raise HTTPException(502, 'Asset download failed')
    if hashlib.sha256(data).hexdigest() != payload.inputAsset.checksumSha256:
        raise HTTPException(422, 'Asset checksum mismatch')
    return predict(data, payload.parameters, payload.runId)
