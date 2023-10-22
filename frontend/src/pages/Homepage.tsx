import React from 'react';
import { styled } from '@mui/system';

const Container = styled('div')({
    maxWidth: '1200px',
    margin: '0 auto',
});

const Header = styled('div')({
    padding: '20px',
    textAlign: 'center',
    backgroundColor: '#f5f5f5',
});

const Block = styled('div')({
    padding: '20px',
    textAlign: 'center',
    marginBottom: '20px',
    boxShadow: '0 4px 8px rgba(0,0,0,0.1)',
});

const Button = styled('button')({
    padding: '10px 20px',
    border: 'none',
    backgroundColor: '#007BFF',
    color: 'white',
    borderRadius: '4px',
    // cursor: 'pointer',
});

function App() {
    return (
        <Container>
            <Header>
                <img src="/img/helix-text-logo.png" alt="Helix Logo" style={{width:"400px"}} />
            </Header>
            <OpenAIBlock />
            <ImageModelsBlock />
            <LanguageModelsBlock />
            <DeploymentBlock />
            <Footer />
        </Container>
    );
}

function OpenAIBlock() {
    return (
        <Block>
            <video autoPlay loop muted  style={{width:"400px"}}>
                <source src="/img/typing.mp4" type="video/mp4"/>
            </video>
            <h2>Open AI 😉</h2>
            <p>Deploy the latest open source models securely into your cloud</p>
            <p>Or let us run them for you</p>
        </Block>
    );
}

function ImageModelsBlock() {
    return (
        <Block>
            <img src="/img/sdxl.png" alt="Stable Diffusion XL" style={{width:"400px"}} />
            <h2>Image models</h2>
            <p>Train your own SDXL customized to your brand or style</p>
            <Button>FINE TUNE SDXL</Button>
        </Block>
    );
}

function LanguageModelsBlock() {
    return (
        <Block>
            <img src="/img/mistral.png" alt="Mistral-7B" style={{width:"400px"}} />
            <h2>Language models</h2>
            <p>Small open source LLMs are beating proprietary models</p>
            <Button>FINE TUNE MISTRAL-7B</Button>
        </Block>
    );
}

function DeploymentBlock() {
    return (
        <Block>
            <img src="/img/servers.png" alt="Servers in a data center" style={{width:"400px"}} />
            <h2>Deployment</h2>
            <ul>
                <li>GPU scheduler</li>
                <li>Smart runners</li>
                <li>Autoscaler</li>
            </ul>
            <Button>DEPLOY ON YOUR INFRA</Button>
        </Block>
    );
}

function Footer() {
    return (
        <Block>
            <img src="/img/github.png" alt="GitHub users collaborating" style={{width:"400px"}} />
            <h2>Clone us on GitHub</h2>
            <p>Bring new models to the open stack</p>
            <a href="https://github.com/helix-ml/helix">GITHUB.COM/HELIX-ML/HELIX</a>
            <Button>JOIN MLOPS.COMMUNITY SLACK</Button>
        </Block>
    );
}

export default App;