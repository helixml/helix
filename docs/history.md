```
152  git clone git@github.com:lukemarsden/axolotl.git
153  cd axolotl/
154  ls
155  git log
156  ls
157  virtualenv venv
158  sudo apt install python3-virtualenv
159  ls
160  virtualenv venv
161  . venv/bin/activate
162  which python
163  pip3 install packaging
164  pip3 install -e '.[flash-attn,deepspeed]'
165  cat requirements
166  cat requirements.txt 
167  cat requirements.txt |grep torch
168  pip install torch==2.0.1
169  pip3 install -e '.[flash-attn,deepspeed]'
170  sudo apt install -y cuda
171  sudo shutdown -r now
172  cd projects/helix
173  ls
174  git clone https://github.com/kohya-ss/sd-scripts
175  ls
176  cd sd-scripts
177  source ../axolotl/venv/bin/activate
178  vim ../models.txt
179  ls
180  fg
181  cd ../
182  ls
183  cd axolotl/
184  ls
185  cat examples/mistral/qlora-instruct.yml
186  git branch
187  git checkout experiments
188  git log
189  git branch
190  git checkout main
191  git merge experiments
192  git push
193  cat examples/mistral/qlora-instruct.yml
194  diff examples/mistral/qlora-instruct.yml examples/mistral/qlora.yml 
195  fg
196  ls
197  fg
198  ls
199  fg
200  ls
201  cd ..
202  ls
203  vim models.txt 
204  ls
205  cd projects/
206  ls
207  cd helix
208  ls
209  cd sd-scripts/
210  ls
211  cat Dockerfile 
212  sudo apt install -y apt-get install -y libgl1-mesa-glx ffmpeg libsm6 libxext6
213  apt-get install -y libgl1-mesa-glx ffmpeg libsm6 libxext6
214  sudo apt-get install -y libgl1-mesa-glx ffmpeg libsm6 libxext6
215  ls
216  cd ..
217  ls
218  vim models.txt 
219  ls
220  wget //storage.googleapis.com/dagger-assets/sdxl_for-sale-signs.zip
221  wget http://storage.googleapis.com/dagger-assets/sdxl_for-sale-signs.zip
222  unzip sdxl_for-sale-signs.zip 
223  cd for-sale-signs/
224  ls
225  cat image1.txt
226  cd ..
227  ls
228  fg
229  ls
230  cd sd-scripts/
231  ls
232  vim Dockerfile 
233  mkdir sdxl
234  cd sdxl
235  cd sdxl; wget https://huggingface.co/stabilityai/stable-diffusion-xl-base-1.0/resolve/main/sd_xl_base_1.0.safetensors
236  history

```

```
 210  cd axolotl/
  211  ls
  212  . venv/bin/activate
  213  pip3 install -e '.[flash-attn,deepspeed]'
  214  accelerate launch -m axolotl.cli.train examples/mistral/qlora.yml
  215  history
```