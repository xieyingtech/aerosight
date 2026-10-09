FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir torch==2.9.1 --index-url https://download.pytorch.org/whl/cpu \
    && pip install --no-cache-dir -r requirements.txt
COPY server.py .
ENV EMBEDDING_HOST=0.0.0.0 EMBEDDING_PORT=6335 HF_HOME=/models
RUN useradd --uid 10001 --create-home embedding && mkdir /models && chown embedding /models
USER embedding
EXPOSE 6335
CMD ["python", "server.py"]
